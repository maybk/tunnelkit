// 首帧渲染前同步应用缓存主题模式，防止深色模式下出现白闪。
;(function () {
  var cfg = null
  try {
    cfg = JSON.parse(localStorage.getItem('tunnelkit:appearance') || 'null')
  } catch {}
  var mode = cfg && cfg.mode
  var prefersDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches
  var dark = mode === 'dark' || (mode !== 'light' && prefersDark)
  var root = document.documentElement
  root.classList.toggle('dark', !!dark)
  root.setAttribute('data-theme', dark ? 'dark' : 'light')
})()
