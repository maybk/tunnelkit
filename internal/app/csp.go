package app

import "net/http"

// ContentSecurityPolicy 定义桌面 WebView 静态资源的安全策略（禁止内联脚本与远程脚本注入）。
const ContentSecurityPolicy = "default-src 'self' wails:; " +
	"script-src 'self' wails:; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob: wails:; " +
	"font-src 'self' data:; " +
	"connect-src 'self' wails: ws: http://localhost:* http://127.0.0.1:*; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"frame-ancestors 'none'"

// WithSecurityHeaders 为静态资源与图标请求注入 CSP 与防嗅探响应头。
func WithSecurityHeaders(inner func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		wrapped := next
		if inner != nil {
			wrapped = inner(next)
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Security-Policy", ContentSecurityPolicy)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			wrapped.ServeHTTP(w, r)
		})
	}
}
