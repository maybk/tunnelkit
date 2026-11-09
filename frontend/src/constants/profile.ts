export const DEFAULT_VPN_PORT = 443

export const CONFIG_TEXT_PLACEHOLDER = `host = vpn.example.com
port = 443
username = your_account
password = your_password
realm = corp
routes = gateway
dns = split
dns-domain = corp.example.com
reconnect = 1`

export const AUTH_TYPES = ['password', 'saml'] as const
export const ROUTE_MODES = ['gateway', 'full', 'none'] as const
export const DNS_MODES = ['split', 'full', 'off'] as const
export const WORKSPACE_TABS = ['status', 'config', 'logs'] as const

export const AUTH_TYPE_OPTIONS = [
  { value: 'password', label: '账号密码' },
  { value: 'saml', label: 'SAML 单点登录' },
] as const

export const ROUTE_MODE_OPTIONS = [
  {
    value: 'gateway',
    label: '智能分流（推荐，仅内网流量走 VPN）',
    description: '按 FortiGate 网关下发的拆隧道路由表转发流量',
  },
  {
    value: 'full',
    label: '全局代理（所有流量均走 VPN）',
    description: '将所有出站流量重定向至 VPN 隧道',
  },
  {
    value: 'none',
    label: '仅建立连接（不自动修改路由）',
    description: '忽略网关下发路由，仅配置下方附加 CIDR 路由',
  },
] as const

export const DNS_MODE_OPTIONS = [
  {
    value: 'split',
    label: '智能分流（推荐，仅内网域名走 VPN）',
    description: '仅将隧道后缀域名解析请求发往 VPN DNS',
  },
  {
    value: 'full',
    label: '全局使用 VPN DNS',
    description: '所有域名解析优先使用 VPN 下发的 DNS 服务器',
  },
  {
    value: 'off',
    label: '保持本机默认 DNS',
    description: '保持宿主机当前 DNS 配置不变',
  },
] as const
