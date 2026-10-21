package vpn

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"tunnelkit/fortivpn"
	"tunnelkit/internal/helperclient"
	"tunnelkit/internal/profile"
	"tunnelkit/internal/storage"
)

var (
	ErrBusy           = errors.New("正在处理上一个连接操作，请稍候")
	ErrInvalidProfile = errors.New("配置 ID 无效")
	ErrNoActivePrompt = errors.New("当前没有待响应的验证提示")
)

// Conn 抽象与运行中 helper 的 IPC 连接（便于单测注入假实现）。
type Conn interface {
	Events() <-chan helperclient.Message
	Answer(ctx context.Context, id, value string, cancel bool) error
	Stop(ctx context.Context) error
	Close() error
}

// Launcher 抽象提权启动与重新接入 helper 的能力。
type Launcher interface {
	Launch(ctx context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error)
	Attach(ctx context.Context, t helperclient.Ticket) (Conn, *fortivpn.Event, error)
}

// DefaultLauncher 是基于 internal/helperclient 的生产环境 Launcher 实现。
type DefaultLauncher struct {
	HelperPath string
}

func NewDefaultLauncher() *DefaultLauncher {
	return &DefaultLauncher{}
}

func (l *DefaultLauncher) Launch(ctx context.Context, req helperclient.LaunchRequest) (Conn, helperclient.Ticket, error) {
	if req.HelperPath == "" && l.HelperPath != "" {
		req.HelperPath = l.HelperPath
	}
	return helperclient.Launch(ctx, req)
}

func (l *DefaultLauncher) Attach(ctx context.Context, t helperclient.Ticket) (Conn, *fortivpn.Event, error) {
	return helperclient.Attach(ctx, t)
}

// Service 编排 VPN 连接生命周期、helper 事件流消费与状态广播。
type Service struct {
	rootDir      string
	profiles     *profile.Store
	tickets      *TicketStore
	launcher     Launcher
	readyTimeout time.Duration

	opRunning atomic.Bool
	// emitMu 串行化“写状态 + 通知监听者”，保证前端与菜单栏收到的状态顺序与发生顺序一致。
	emitMu        sync.Mutex
	mu            sync.RWMutex
	state         State
	conn          Conn
	streamDone    <-chan struct{}
	opDone        chan struct{}
	cancelConnect context.CancelFunc
	listener      func(State)
}

func NewService(rootDir string, profiles *profile.Store, launcher Launcher) *Service {
	if profiles == nil {
		panic("vpn.NewService: profiles must not be nil")
	}
	if launcher == nil {
		launcher = NewDefaultLauncher()
	}
	return &Service{
		rootDir:      rootDir,
		profiles:     profiles,
		tickets:      NewTicketStore(rootDir),
		launcher:     launcher,
		readyTimeout: 60 * time.Second,
		state:        State{Status: StatusDisconnected, Phase: fortivpn.PhaseDisconnected},
	}
}

// SetListener 注册状态变更监听函数；监听函数会被同步调用，不应阻塞。
func (s *Service) SetListener(fn func(State)) {
	s.mu.Lock()
	s.listener = fn
	s.mu.Unlock()
}

// State 返回当前 VPN 状态。
func (s *Service) State() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// Cancel 中止正在进行的连接；没有进行中的连接时无操作。
func (s *Service) Cancel() {
	s.mu.Lock()
	cancel := s.cancelConnect
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Answer 向当前活动 helper 提交 OTP / FTM / SAML 交互验证响应。
func (s *Service) Answer(ctx context.Context, promptID, value string, cancel bool) error {
	s.mu.RLock()
	conn := s.conn
	s.mu.RUnlock()
	if conn == nil {
		return ErrNoActivePrompt
	}
	if cancel {
		s.Cancel()
	}
	return conn.Answer(ctx, promptID, value, cancel)
}

// Log 读取指定配置的脱敏连接日志。
func (s *Service) Log(profileID string) (LogView, error) {
	if !profile.IsValidID(profileID) {
		return LogView{}, ErrInvalidProfile
	}
	tail, err := storage.TailFile(storage.LogPath(s.rootDir, profileID), storage.DefaultTailBytes)
	if err != nil {
		return LogView{}, err
	}
	view := LogView{ProfileID: profileID, Content: tail.Content, Exists: tail.Exists}
	if tail.Exists {
		view.UpdatedAt = tail.ModTime.Format(time.RFC3339)
	}
	return view, nil
}

// ClearLog 清空指定配置的连接日志。
func (s *Service) ClearLog(profileID string) error {
	if !profile.IsValidID(profileID) {
		return ErrInvalidProfile
	}
	return storage.ResetFile(storage.LogPath(s.rootDir, profileID))
}

// RemoveLog 删除指定配置的连接日志文件（删除配置时调用）。
func (s *Service) RemoveLog(profileID string) error {
	if !profile.IsValidID(profileID) {
		return ErrInvalidProfile
	}
	err := os.Remove(storage.LogPath(s.rootDir, profileID))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Service) begin() (func(), error) {
	if !s.opRunning.CompareAndSwap(false, true) {
		return nil, ErrBusy
	}
	done := make(chan struct{})
	s.mu.Lock()
	s.opDone = done
	s.mu.Unlock()
	return func() {
		s.opRunning.Store(false)
		close(done)
	}, nil
}

func (s *Service) setState(next State) State {
	s.emitMu.Lock()
	defer s.emitMu.Unlock()

	s.mu.Lock()
	s.state = next
	listener := s.listener
	s.mu.Unlock()

	if listener != nil {
		listener(next)
	}
	return next
}

func (s *Service) detachConn() {
	s.mu.Lock()
	conn := s.conn
	s.conn = nil
	s.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}
