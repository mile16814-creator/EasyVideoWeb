const state = {
  categories: [],
  videos: [],
  posters: [],
  activeCategory: "all",
  currentPage: 1,
  pageSize: 12,
  posterIndex: 0,
  posterTimer: null
}

async function loadFrontendConfig() {
  try {
    const res = await fetch("/api/app-config")
    if (!res.ok) return
    const cfg = await res.json()
    const size = cfg && cfg.pagination && Number(cfg.pagination.videosPerPage)
    if (Number.isFinite(size) && size > 0) {
      state.pageSize = size
    }
  } catch (_) {}
}

const elements = {
  categoryTabs: document.getElementById("categoryTabs"),
  videoGrid: document.getElementById("videoGrid"),
  posterGrid: document.getElementById("posterGrid"),
  searchInput: document.getElementById("searchInput"),
  searchBtn: document.getElementById("searchBtn"),
  pagination: document.getElementById("pagination"),
  header: document.querySelector(".boke-header"),
  sidebar: document.querySelector(".sidebar"),
  userEntry: document.getElementById("userEntry"),
  rankList: document.getElementById("rankList"),
  uploadBtn: document.getElementById("uploadBtn"),
  postUploadBtn: document.getElementById("postUploadBtn"),
  // Modal Elements
  loginModal: document.getElementById("loginModal"),
  closeModal: document.querySelector(".close-modal"),
  loginContainer: document.getElementById("loginContainer"),
  registerContainer: document.getElementById("registerContainer"),
  toRegisterLink: document.getElementById("toRegisterLink"),
  toLoginLink: document.getElementById("toLoginLink"),
  // Inputs
  loginEmail: document.getElementById("loginEmail"),
  loginPwd: document.getElementById("loginPwd"),
  regEmail: document.getElementById("regEmail"),
  regCode: document.getElementById("regCode"),
  regPwd: document.getElementById("regPwd"),
  codeInput: document.getElementById("codeInput"),
  // Buttons
  doLoginBtn: document.getElementById("doLoginBtn"),
  forgotLink: document.getElementById("forgotLink"),
  backToPwd: document.getElementById("backToPwd"),
  sendCodeBtn: document.getElementById("sendCodeBtn"),
  codeLoginBtn: document.getElementById("codeLoginBtn"),
  getVerCodeBtn: document.getElementById("getVerCodeBtn"),
  doRegBtn: document.getElementById("doRegBtn"),
  // Messages
  loginMsg: document.getElementById("loginMsg"),
  regMsg: document.getElementById("regMsg"),
  codeLoginArea: document.getElementById("codeLoginArea")
}

// User Login State Management
const guestAvatarSvg = `data:image/svg+xml;utf8,${encodeURIComponent(
  "<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'><circle cx='32' cy='32' r='32' fill='%23E3E5E7'/><circle cx='32' cy='24' r='10' fill='%239499A0'/><path d='M12 54c4-10 16-14 20-14s16 4 20 14' fill='%239499A0'/></svg>"
)}`

function applyUserAvatar(user) {
  const avatarEl = elements.userEntry.querySelector(".avatar-placeholder")
  if (user && user.avatarUrl) {
    avatarEl.textContent = ""
    avatarEl.style.backgroundImage = `url(${user.avatarUrl})`
    avatarEl.style.backgroundSize = "cover"
    avatarEl.style.backgroundPosition = "center"
    avatarEl.style.backgroundRepeat = "no-repeat"
    avatarEl.style.backgroundColor = "transparent"
  } else {
    const nickname = user && user.nickname ? user.nickname : "U"
    avatarEl.textContent = nickname ? nickname[0].toUpperCase() : "U"
    avatarEl.style.background = "#00AEEC"
    avatarEl.style.color = "#fff"
    avatarEl.style.backgroundImage = "none"
  }
  avatarEl.title = (user && user.nickname) || "User"
  elements.userEntry.onclick = () => {
    window.location.href = "/user.html"
  }
}

function applyLoggedOutAvatar() {
  const avatarEl = elements.userEntry.querySelector(".avatar-placeholder")
  avatarEl.textContent = ""
  avatarEl.style.background = "#E3E5E7"
  avatarEl.style.color = "#9499A0"
  avatarEl.style.backgroundImage = `url("${guestAvatarSvg}")`
  avatarEl.style.backgroundSize = "70%"
  avatarEl.style.backgroundPosition = "center"
  avatarEl.style.backgroundRepeat = "no-repeat"
  elements.userEntry.onclick = showLoginModal
}

function showBanModal(reason, bannedUntil, remainingSeconds, onClose) {
  const sec = Number(remainingSeconds) || 0
  const days = Math.floor(sec / 86400)
  const hours = Math.floor((sec % 86400) / 3600)
  const mins = Math.floor((sec % 3600) / 60)
  let remainText = "剩余时间："
  if (days > 0) remainText += days + " 天 "
  if (hours > 0) remainText += hours + " 小时 "
  remainText += mins + " 分钟"
  const msg = "您的账号已被封禁。\n\n封号原因：" + (reason || "违反社区规范") + "\n\n解封时间：" + (bannedUntil || "") + "\n" + (sec > 0 ? remainText : "")
  const overlay = document.createElement("div")
  overlay.id = "banModalOverlay"
  overlay.style.cssText = "position:fixed;inset:0;background:rgba(0,0,0,0.6);display:flex;align-items:center;justify-content:center;z-index:99999;padding:20px;box-sizing:border-box;"
  const box = document.createElement("div")
  box.style.cssText = "background:#fff;border-radius:12px;padding:24px;max-width:400px;width:100%;box-shadow:0 8px 32px rgba(0,0,0,0.2);"
  box.innerHTML = "<h3 style='margin:0 0 12px;color:#c00;'>账号已封禁</h3><p style='margin:0 0 16px;white-space:pre-wrap;line-height:1.5;'>" + msg.replace(/</g, "&lt;").replace(/\n/g, "<br>") + "</p><button id='banModalOk' style='padding:8px 20px;background:#00AEEC;color:#fff;border:none;border-radius:6px;cursor:pointer;'>确定</button>"
  overlay.appendChild(box)
  document.body.appendChild(overlay)
  box.querySelector("#banModalOk").onclick = () => {
    document.body.removeChild(overlay)
    if (typeof onClose === "function") onClose()
    localStorage.removeItem("authToken")
    localStorage.removeItem("user")
    if (window.applyLoggedOutAvatar) applyLoggedOutAvatar()
    location.reload()
  }
}

async function checkUserPunishment(token) {
  if (!token) return
  try {
    const res = await fetch("/api/user-punishment", { headers: { Authorization: "Bearer " + token } })
    if (!res.ok) return
    const data = await res.json()
    if (data.banned && data.banned.reason) {
      showBanModal(data.banned.reason, data.banned.bannedUntil, data.banned.remainingSeconds, () => {
        localStorage.removeItem("authToken")
        localStorage.removeItem("user")
      })
      return
    }
    if (data.muted && data.muted.reason && document.getElementById("muteBanner") == null) {
      const banner = document.createElement("div")
      banner.id = "muteBanner"
      banner.style.cssText = "position:fixed;top:0;left:0;right:0;background:#f59e0b;color:#fff;padding:10px 20px;text-align:center;z-index:99998;font-size:14px;"
      banner.textContent = "您已被禁言至 " + (data.muted.mutedUntil || "") + "，原因：" + data.muted.reason + "。禁言期间无法评论与发布视频/帖子。"
      document.body.appendChild(banner)
    }
  } catch (e) {}
}

async function checkLoginState() {
  const legacyToken = localStorage.getItem("token")
  if (legacyToken && !localStorage.getItem("authToken")) {
    localStorage.setItem("authToken", legacyToken)
    localStorage.removeItem("token")
  }
  const token = localStorage.getItem("authToken")
  const userStr = localStorage.getItem("user")
  let cachedUser = null
  const clearAuthState = () => {
    localStorage.removeItem("authToken")
    localStorage.removeItem("user")
  }
  if (token) {
    if (userStr) {
      try {
        const user = JSON.parse(userStr)
        cachedUser = user
        if (user) applyUserAvatar(user)
      } catch (e) {
        localStorage.removeItem("user")
      }
    }
    try {
      const res = await fetch("/api/profile", {
        headers: { Authorization: `Bearer ${token}` }
      })
      if (res.ok) {
        const profile = await res.json()
        localStorage.setItem("user", JSON.stringify(profile))
        applyUserAvatar(profile)
        checkUserPunishment(token)
        return
      }
      if (res.status === 403) {
        const data = await res.json().catch(() => ({}))
        if (data.code === "banned") {
          showBanModal(data.reason, data.bannedUntil, data.remainingSeconds, clearAuthState)
          applyLoggedOutAvatar()
          return
        }
      }
      if (res.status === 401 || res.status === 403) {
        clearAuthState()
        applyLoggedOutAvatar()
        return
      }
    } catch (e) {
    }
    if (cachedUser) {
      applyUserAvatar(cachedUser)
      return
    }
    clearAuthState()
  }
  applyLoggedOutAvatar()
}

// Upload Button Handler
if (elements.uploadBtn) {
  elements.uploadBtn.onclick = () => {
    const token = localStorage.getItem("authToken")
    if (token) {
       window.location.href = "/creator.html"
    } else {
       showLoginModal()
    }
  }
}
if (elements.postUploadBtn) {
  elements.postUploadBtn.onclick = () => {
    const token = localStorage.getItem("authToken")
    if (token) {
      window.location.href = "/post_create.html"
    } else {
      showLoginModal()
    }
  }
}

// Modal Logic
function showLoginModal() {
  elements.loginModal.style.display = "flex"
  showLoginView()
}

function hideLoginModal() {
  elements.loginModal.style.display = "none"
  // Clear inputs
  elements.loginEmail.value = ""
  elements.loginPwd.value = ""
  if (elements.codeInput) elements.codeInput.value = ""
  elements.regEmail.value = ""
  elements.regCode.value = ""
  elements.regPwd.value = ""
  elements.loginMsg.textContent = ""
  elements.regMsg.textContent = ""
  showPasswordLogin()
}

function showLoginView() {
  elements.loginContainer.style.display = "block"
  elements.registerContainer.style.display = "none"
  showPasswordLogin()
}

function showRegisterView() {
  elements.loginContainer.style.display = "none"
  elements.registerContainer.style.display = "block"
}

if (elements.closeModal) elements.closeModal.onclick = hideLoginModal
if (elements.toRegisterLink) elements.toRegisterLink.onclick = showRegisterView
if (elements.toLoginLink) elements.toLoginLink.onclick = showLoginView

const passwordGroup = elements.loginPwd ? elements.loginPwd.closest(".form-group") : null

function showPasswordLogin() {
  if (passwordGroup) passwordGroup.style.display = ""
  if (elements.doLoginBtn) elements.doLoginBtn.style.display = ""
  if (elements.forgotLink) elements.forgotLink.style.display = ""
  if (elements.codeLoginArea) elements.codeLoginArea.style.display = "none"
  if (elements.loginMsg) elements.loginMsg.textContent = ""
}

function showCodeLogin() {
  if (passwordGroup) passwordGroup.style.display = "none"
  if (elements.doLoginBtn) elements.doLoginBtn.style.display = "none"
  if (elements.forgotLink) elements.forgotLink.style.display = "none"
  if (elements.codeLoginArea) elements.codeLoginArea.style.display = "block"
  if (elements.loginMsg) elements.loginMsg.textContent = ""
}

if (elements.forgotLink) elements.forgotLink.onclick = showCodeLogin
if (elements.backToPwd) elements.backToPwd.onclick = showPasswordLogin

function initLoginFromUrl() {
  const params = new URLSearchParams(window.location.search)
  if (params.get("login") === "1") {
    const redirect = params.get("redirect")
    if (redirect) {
      try {
        const decoded = decodeURIComponent(redirect)
        if (decoded.startsWith("/")) {
          sessionStorage.setItem("postLoginRedirect", decoded)
        }
      } catch (e) {
      }
    }
    showLoginModal()
  }
}

// Login Action
elements.doLoginBtn.onclick = async () => {
  const email = elements.loginEmail.value.trim()
  const password = elements.loginPwd.value.trim()
  if (!email || !password) {
    elements.loginMsg.textContent = "请输入邮箱和密码"
    elements.loginMsg.className = "error-msg"
    return
  }
  
  // Disable button to prevent double submit
  elements.doLoginBtn.disabled = true
  elements.doLoginBtn.textContent = "登录中..."

  try {
    const res = await fetch("/api/login", { // Use /api/login as in login.html
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password })
    })
    
    if (res.status === 403) {
      const data = await res.json().catch(() => ({}))
      if (data.code === "banned") {
        showBanModal(data.reason, data.bannedUntil, data.remainingSeconds, () => {})
        elements.doLoginBtn.disabled = false
        elements.doLoginBtn.textContent = "密码登录"
        return
      }
    }
    if (!res.ok) {
       const text = await res.text()
       throw new Error(text || "登录失败")
    }

    const data = await res.json()
    if (!data.token) throw new Error("登录失败")
    
    // Login Success
    localStorage.setItem("authToken", data.token)
    if (data.user) {
      localStorage.setItem("user", JSON.stringify(data.user))
    } else {
      try {
        const profileRes = await fetch("/api/profile", {
          headers: { Authorization: `Bearer ${data.token}` }
        })
        if (profileRes.ok) {
          const profile = await profileRes.json()
          localStorage.setItem("user", JSON.stringify(profile))
        }
      } catch (e) {
      }
    }
    const redirectUrl = sessionStorage.getItem("postLoginRedirect")
    if (redirectUrl) {
      sessionStorage.removeItem("postLoginRedirect")
      window.location.href = redirectUrl
      return
    }
    location.reload()
    
  } catch (err) {
    elements.loginMsg.textContent = err.message
    elements.loginMsg.className = "error-msg"
    elements.doLoginBtn.disabled = false
    elements.doLoginBtn.textContent = "密码登录"
  }
}

// Register Actions
elements.getVerCodeBtn.onclick = async () => {
  const email = elements.regEmail.value.trim()
  if (!email) {
    elements.regMsg.textContent = "请输入邮箱"
    elements.regMsg.className = "error-msg"
    return
  }

  elements.getVerCodeBtn.disabled = true
  elements.getVerCodeBtn.textContent = "发送中..."
  elements.regMsg.textContent = ""

  try {
    const res = await fetch("/api/send-code", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email })
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(text || "发送失败")
    }
    elements.regMsg.textContent = "验证码已发送，请查收邮件"
    elements.regMsg.className = "success-msg"

    let count = 60
    const timer = setInterval(() => {
      count--
      elements.getVerCodeBtn.textContent = `${count}s`
      if (count <= 0) {
        clearInterval(timer)
        elements.getVerCodeBtn.disabled = false
        elements.getVerCodeBtn.textContent = "获取验证码"
      }
    }, 1000)
  } catch (err) {
    elements.regMsg.textContent = err.message
    elements.regMsg.className = "error-msg"
    elements.getVerCodeBtn.disabled = false
    elements.getVerCodeBtn.textContent = "获取验证码"
  }
}

elements.doRegBtn.onclick = async () => {
  const email = elements.regEmail.value.trim()
  const code = elements.regCode.value.trim()
  const password = elements.regPwd.value.trim()

  if (!email || !code || !password) {
    elements.regMsg.textContent = "请填写完整信息"
    elements.regMsg.className = "error-msg"
    return
  }

  elements.doRegBtn.disabled = true
  elements.doRegBtn.textContent = "注册中..."

  try {
    const res = await fetch("/api/verify-code", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, code, password })
    })

    if (!res.ok) {
      const text = await res.text()
      throw new Error(text || "注册失败")
    }

    const data = await res.json()
    if (!data.token) throw new Error("注册失败")

    localStorage.setItem("authToken", data.token)
    if (data.user) {
      localStorage.setItem("user", JSON.stringify(data.user))
    }

    elements.regMsg.textContent = "注册成功，即将跳转..."
    elements.regMsg.className = "success-msg"
    setTimeout(() => {
      const redirectUrl = sessionStorage.getItem("postLoginRedirect")
      if (redirectUrl) {
        sessionStorage.removeItem("postLoginRedirect")
        window.location.href = redirectUrl
        return
      }
      location.reload()
    }, 500)

  } catch (err) {
    elements.regMsg.textContent = err.message
    elements.regMsg.className = "error-msg"
    elements.doRegBtn.disabled = false
    elements.doRegBtn.textContent = "注册并登录"
  }
}

if (elements.sendCodeBtn) {
  elements.sendCodeBtn.onclick = async () => {
    const email = elements.loginEmail.value.trim()
    if (!email) {
      elements.loginMsg.textContent = "请输入邮箱"
      elements.loginMsg.className = "error-msg"
      return
    }
    if (!/^\S+@\S+\.\S+$/.test(email)) {
      elements.loginMsg.textContent = "请输入有效邮箱"
      elements.loginMsg.className = "error-msg"
      return
    }
    elements.sendCodeBtn.disabled = true
    elements.sendCodeBtn.textContent = "发送中..."
    elements.loginMsg.textContent = ""
    try {
      const res = await fetch("/api/login-code/send", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email })
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "发送失败")
      }
      elements.loginMsg.textContent = "验证码已发送"
      elements.loginMsg.className = "success-msg"
      let count = 60
      const timer = setInterval(() => {
        count--
        elements.sendCodeBtn.textContent = `${count}s`
        if (count <= 0) {
          clearInterval(timer)
          elements.sendCodeBtn.disabled = false
          elements.sendCodeBtn.textContent = "获取验证码"
        }
      }, 1000)
    } catch (err) {
      elements.loginMsg.textContent = err.message
      elements.loginMsg.className = "error-msg"
      elements.sendCodeBtn.disabled = false
      elements.sendCodeBtn.textContent = "获取验证码"
    }
  }
}

if (elements.codeLoginBtn) {
  elements.codeLoginBtn.onclick = async () => {
    const email = elements.loginEmail.value.trim()
    const code = elements.codeInput ? elements.codeInput.value.trim() : ""
    if (!email || !code) {
      elements.loginMsg.textContent = "请输入邮箱和验证码"
      elements.loginMsg.className = "error-msg"
      return
    }
    elements.codeLoginBtn.disabled = true
    elements.codeLoginBtn.textContent = "登录中..."
    elements.loginMsg.textContent = ""
    try {
      const res = await fetch("/api/login-code/verify", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, code })
      })
      if (res.status === 403) {
        const data = await res.json().catch(() => ({}))
        if (data.code === "banned") {
          showBanModal(data.reason, data.bannedUntil, data.remainingSeconds, () => {})
          elements.codeLoginBtn.disabled = false
          elements.codeLoginBtn.textContent = "验证码登录"
          return
        }
      }
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || "登录失败")
      }
      const data = await res.json()
      if (!data.token) throw new Error("登录失败")
      localStorage.setItem("authToken", data.token)
      if (data.user) {
        localStorage.setItem("user", JSON.stringify(data.user))
      }
      const redirectUrl = sessionStorage.getItem("postLoginRedirect")
      if (redirectUrl) {
        sessionStorage.removeItem("postLoginRedirect")
        window.location.href = redirectUrl
        return
      }
      location.reload()
    } catch (err) {
      elements.loginMsg.textContent = err.message
      elements.loginMsg.className = "error-msg"
      elements.codeLoginBtn.disabled = false
      elements.codeLoginBtn.textContent = "验证码登录"
    }
  }
}

// 本月最热视频（固定 10 条，接口每12小时刷新，只统计本月1号0点之后发布的视频）
function renderRankings(rankingList) {
  if (!elements.rankList) return
  const list = (Array.isArray(rankingList) ? rankingList : []).slice(0, 10)
  const rankTitle = (window.evwT && window.evwT("index.rankTitle")) || "本月最热视频"
  if (list.length === 0) {
    elements.rankList.innerHTML = "<h3>" + rankTitle + "</h3>"
    return
  }
  elements.rankList.innerHTML = "<h3>" + rankTitle + "</h3>"
  list.forEach((video, index) => {
    const item = document.createElement("div")
    item.className = "rank-item"

    const numSpan = document.createElement("span")
    numSpan.className = `rank-num ${index < 3 ? "top" : ""}`
    numSpan.textContent = index + 1

    const titleSpan = document.createElement("span")
    titleSpan.className = "rank-title"
    const title = video.title ? String(video.title).trim() : ""
    titleSpan.textContent = title
    titleSpan.title = title

    item.appendChild(numSpan)
    item.appendChild(titleSpan)
    item.onclick = () => {
      window.location.href = `/player.html?id=${encodeURIComponent(video.id)}`
    }
    elements.rankList.appendChild(item)
  })
}

function formatDuration(seconds) {
  if (!seconds) return "-"
  const total = Math.floor(seconds)
  const m = String(Math.floor(total / 60)).padStart(2, "0")
  const s = String(total % 60).padStart(2, "0")
  return `${m}:${s}`
}

function formatCount(n) {
  const num = Number(n || 0)
  if (num >= 10000) {
    return `${(num / 10000).toFixed(1).replace(/\.0$/, "")}万`
  }
  return String(num)
}

function renderPosters() {
  if (!elements.posterGrid) return
  elements.posterGrid.innerHTML = ""
  elements.posterGrid.style.display = ""
  if (!Array.isArray(state.posters) || state.posters.length === 0) {
    const track = document.createElement("div")
    track.className = "poster-carousel-track"
    track.style.background = "#1a1a2e"
    elements.posterGrid.appendChild(track)
    return
  }
  state.posterIndex = 0
  if (state.posterTimer) {
    clearInterval(state.posterTimer)
    state.posterTimer = null
  }

  const track = document.createElement("div")
  track.className = "poster-carousel-track"

  state.posters.forEach((poster) => {
    const slide = document.createElement("div")
    slide.className = "poster-carousel-slide"
    const href = poster.linkUrl && String(poster.linkUrl).trim() ? poster.linkUrl.trim() : "javascript:void(0)"
    const link = document.createElement("a")
    link.href = href
    if (poster.openInNewTab && href !== "javascript:void(0)") {
      link.target = "_blank"
      link.rel = "noopener noreferrer"
    }
    link.innerHTML = `<img src="${poster.imageUrl || ""}" alt="poster">`
    slide.appendChild(link)
    track.appendChild(slide)
  })
  elements.posterGrid.appendChild(track)

  if (state.posters.length > 1) {
    const prevBtn = document.createElement("button")
    prevBtn.className = "poster-carousel-btn poster-prev"
    prevBtn.innerHTML = `<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>`
    prevBtn.onclick = (e) => { e.preventDefault(); slidePoster(-1) }

    const nextBtn = document.createElement("button")
    nextBtn.className = "poster-carousel-btn poster-next"
    nextBtn.innerHTML = `<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 6 15 12 9 18"/></svg>`
    nextBtn.onclick = (e) => { e.preventDefault(); slidePoster(1) }

    elements.posterGrid.appendChild(prevBtn)
    elements.posterGrid.appendChild(nextBtn)

    const dots = document.createElement("div")
    dots.className = "poster-carousel-dots"
    state.posters.forEach((_, i) => {
      const dot = document.createElement("button")
      dot.className = "poster-carousel-dot" + (i === 0 ? " active" : "")
      dot.onclick = (e) => { e.preventDefault(); goToPoster(i) }
      dots.appendChild(dot)
    })
    elements.posterGrid.appendChild(dots)

    state.posterTimer = setInterval(() => slidePoster(1), 5000)
  }
}

function slidePoster(dir) {
  const len = state.posters.length
  if (len <= 1) return
  state.posterIndex = (state.posterIndex + dir + len) % len
  updatePosterPosition()
}

function goToPoster(idx) {
  state.posterIndex = idx
  updatePosterPosition()
  if (state.posterTimer) {
    clearInterval(state.posterTimer)
    state.posterTimer = setInterval(() => slidePoster(1), 5000)
  }
}

function updatePosterPosition() {
  const grid = elements.posterGrid
  if (!grid) return
  const track = grid.querySelector(".poster-carousel-track")
  if (track) {
    track.style.transform = `translateX(-${state.posterIndex * 100}%)`
  }
  grid.querySelectorAll(".poster-carousel-dot").forEach((d, i) => {
    d.classList.toggle("active", i === state.posterIndex)
  })
}

function categoryName(id) {
  if (id === "all" && window.evwT) return window.evwT("common.all")
  const found = state.categories.find((cat) => cat.id === id)
  return found ? found.name : id
}

function renderCategories() {
  elements.categoryTabs.innerHTML = ""
  state.categories.forEach((cat) => {
    const btn = document.createElement("button")
    btn.className = `category ${state.activeCategory === cat.id ? "active" : ""}`
    btn.textContent = (cat.id === "all" && window.evwT) ? window.evwT("common.all") : cat.name
    btn.addEventListener("click", () => {
      state.activeCategory = cat.id
      renderCategories()
      loadVideos(elements.searchInput.value.trim())
    })
    elements.categoryTabs.appendChild(btn)
  })
}

function totalPages() {
  return Math.ceil(state.videos.length / state.pageSize)
}

function setPage(nextPage) {
  const total = totalPages()
  if (total === 0) {
    if (state.currentPage !== 1) {
      state.currentPage = 1
    }
    renderPagination()
    return
  }
  const target = Math.min(Math.max(1, nextPage), total)
  if (state.currentPage === target) {
    renderPagination()
    return
  }
  state.currentPage = target
  renderVideos()
}

async function submitReport(type, id, title) {
  const token = localStorage.getItem("authToken")
  if (!token) {
    showLoginModal()
    return
  }
  const reason = prompt(`请输入举报「${title || ""}」的理由（可选）：`)
  if (reason === null) return
  try {
    const url = type === "video" ? "/api/videos/report" : "/api/posts/report"
    const body = type === "video" ? { videoId: id, reason } : { postId: id, reason }
    const res = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
      body: JSON.stringify(body)
    })
    if (res.status === 401) {
      showLoginModal()
      return
    }
    if (!res.ok) {
      const errText = await res.text()
      alert(errText || "举报失败")
      return
    }
    await res.json()
    alert("举报成功")
  } catch (e) {
    alert("举报失败")
  }
}

function renderVideos() {
  elements.videoGrid.innerHTML = ""
  if (state.videos.length === 0) {
    const empty = document.createElement("div")
    empty.className = "empty"
    empty.textContent = (window.evwT && window.evwT("index.noVideos")) || "暂无视频"
    elements.videoGrid.appendChild(empty)
    state.currentPage = 1
    renderPagination()
    return
  }
  const total = totalPages()
  if (state.currentPage < 1) {
    state.currentPage = 1
  }
  if (state.currentPage > total) {
    state.currentPage = total
  }
  const start = (state.currentPage - 1) * state.pageSize
  const end = start + state.pageSize
  const pageVideos = state.videos.slice(start, end)
  pageVideos.forEach((video) => {
    const card = document.createElement("div")
    card.className = "card"
    const thumb = document.createElement("div")
    thumb.className = "thumb"
    if (video.thumbUrl) {
      thumb.style.backgroundImage = `url(${video.thumbUrl})`
    }
    const durationTag = document.createElement("span")
    durationTag.className = "thumb-duration"
    durationTag.textContent = formatDuration(video.durationSec)
    thumb.appendChild(durationTag)
    const body = document.createElement("div")
    body.className = "card-body"
    const title = document.createElement("div")
    title.className = "card-title"
    title.textContent = video.title
    const cat = document.createElement("span")
    cat.className = "card-category"
    cat.textContent = `· ${categoryName(video.category)}`
    title.appendChild(cat)
    if (video.tags && video.tags.length > 0) {
      const tags = document.createElement("span")
      tags.className = "card-tags"
      tags.textContent = `· ${video.tags.join(" / ")}`
      title.appendChild(tags)
    }
    const meta = document.createElement("div")
    meta.className = "card-meta"
    const metaLine = document.createElement("div")
    metaLine.className = "card-meta-line"
    const author = document.createElement("span")
    author.textContent = video.authorNickname || "UP主"
    const views = document.createElement("span")
    views.textContent = `${formatCount(video.views)} 次观看`
    metaLine.appendChild(author)
    metaLine.appendChild(views)
    meta.appendChild(metaLine)
    body.appendChild(title)
    body.appendChild(meta)
    card.appendChild(thumb)
    card.appendChild(body)
    card.addEventListener("click", () => {
      window.location.href = `/player.html?id=${encodeURIComponent(video.id)}`
    })
    elements.videoGrid.appendChild(card)
  })
  renderPagination()
}

function ensurePaginationElements() {
  if (elements.pagination) return
  const mainColumn = document.querySelector(".main-column")
  if (!mainColumn) return
  let wrapper = mainColumn.querySelector(".pagination-wrapper")
  if (!wrapper) {
    wrapper = document.createElement("div")
    wrapper.className = "pagination-wrapper"
    mainColumn.appendChild(wrapper)
  }
  const pagination = document.createElement("section")
  pagination.className = "pagination"
  pagination.id = "pagination"
  wrapper.appendChild(pagination)
  elements.pagination = pagination
}

function renderPagination() {
  ensurePaginationElements()
  if (!elements.pagination) return
  const wrapper = elements.pagination.closest(".pagination-wrapper")
  elements.pagination.innerHTML = ""
  if (!Array.isArray(state.videos)) {
    state.videos = []
  }
  const total = totalPages()

  if (wrapper) wrapper.style.display = ""

  if (total === 0) {
    const createBtn = (text, onClick, className = "", disabled = false) => {
      const btn = document.createElement("button")
      btn.className = `page-btn ${className}`.trim()
      btn.textContent = text
      btn.disabled = disabled
      if (onClick && !disabled) {
        btn.addEventListener("click", onClick)
      }
      return btn
    }
    var prevT = (window.evwT && window.evwT("common.pagination.prev")) || "上一页"
    var nextT = (window.evwT && window.evwT("common.pagination.next")) || "下一页"
    var jumpT = (window.evwT && window.evwT("common.pagination.jumpTo")) || "跳至"
    var pageT = (window.evwT && window.evwT("common.pagination.page")) || "页"
    elements.pagination.appendChild(createBtn(prevT, null, "prev-btn", true))
    elements.pagination.appendChild(createBtn("1", null, "active", false))
    elements.pagination.appendChild(createBtn(nextT, null, "next-btn", true))
    const jumpContainer = document.createElement("div")
    jumpContainer.className = "page-jump"
    const jumpText1 = document.createElement("span")
    jumpText1.textContent = jumpT
    jumpContainer.appendChild(jumpText1)
    const jumpInput = document.createElement("input")
    jumpInput.type = "text"
    jumpInput.className = "jump-input"
    jumpInput.readOnly = true
    jumpInput.placeholder = ""
    jumpInput.tabIndex = -1
    jumpInput.style.pointerEvents = "none"
    jumpInput.style.opacity = "1"
    jumpInput.style.background = "#fff"
    jumpContainer.appendChild(jumpInput)
    const jumpText2 = document.createElement("span")
    jumpText2.textContent = pageT
    jumpContainer.appendChild(jumpText2)
    elements.pagination.appendChild(jumpContainer)
    updateSidebarLayout()
    return
  }

  var prevT = (window.evwT && window.evwT("common.pagination.prev")) || "上一页"
  var nextT = (window.evwT && window.evwT("common.pagination.next")) || "下一页"
  var jumpT = (window.evwT && window.evwT("common.pagination.jumpTo")) || "跳至"
  var pageT = (window.evwT && window.evwT("common.pagination.page")) || "页"

  const createBtn = (text, onClick, className = "", disabled = false) => {
    const btn = document.createElement("button")
    btn.className = `page-btn ${className}`.trim()
    btn.textContent = text
    btn.disabled = disabled
    if (onClick && !disabled) {
      btn.addEventListener("click", onClick)
    }
    return btn
  }

  elements.pagination.appendChild(createBtn(prevT, () => setPage(state.currentPage - 1), "prev-btn", state.currentPage === 1))

  const windowSize = 2
  let pages = []

  if (total <= 7) {
    for (let i = 1; i <= total; i++) pages.push(i)
  } else {
    let left = state.currentPage - windowSize
    let right = state.currentPage + windowSize

    if (left <= 2) {
      left = 1
      right = Math.min(total, 5 + (windowSize * 2))
    }

    if (right >= total - 1) {
      right = total
      left = Math.max(1, total - (4 + (windowSize * 2)))
    }

    if (left > 1) {
      pages.push(1)
      if (left > 2) pages.push("...")
    }

    for (let i = Math.max(1, left); i <= Math.min(total, right); i++) {
      pages.push(i)
    }

    if (right < total) {
      if (right < total - 1) pages.push("...")
      pages.push(total)
    }
  }

  pages = [...new Set(pages)]

  pages.forEach((p) => {
    if (p === "...") {
      const span = document.createElement("span")
      span.className = "page-dots"
      span.textContent = "..."
      elements.pagination.appendChild(span)
    } else {
      elements.pagination.appendChild(createBtn(String(p), () => setPage(p), p === state.currentPage ? "active" : ""))
    }
  })

  elements.pagination.appendChild(createBtn(nextT, () => setPage(state.currentPage + 1), "next-btn", state.currentPage === total))

  const jumpContainer = document.createElement("div")
  jumpContainer.className = "page-jump"

  const jumpText1 = document.createElement("span")
  jumpText1.textContent = jumpT
  jumpContainer.appendChild(jumpText1)

  const jumpInput = document.createElement("input")
  jumpInput.type = "text"
  jumpInput.className = "jump-input"
  jumpInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      const val = parseInt(jumpInput.value)
      if (!isNaN(val) && val >= 1 && val <= total) {
        setPage(val)
      } else {
        jumpInput.value = ""
      }
    }
  })
  jumpContainer.appendChild(jumpInput)

  const jumpText2 = document.createElement("span")
  jumpText2.textContent = pageT
  jumpContainer.appendChild(jumpText2)

  elements.pagination.appendChild(jumpContainer)

  updateSidebarLayout()
}

function updateSidebarLayout() {
  if (!elements.sidebar || !elements.header || !elements.pagination) {
    return
  }
  if (window.matchMedia("(max-width: 1080px)").matches) {
    elements.sidebar.style.top = ""
    elements.sidebar.style.bottom = ""
    return
  }
  const headerRect = elements.header.getBoundingClientRect()
  const paginationRect = elements.pagination.getBoundingClientRect()
  const topOffset = Math.round(headerRect.bottom + 20)
  const bottomOffset = Math.round(window.innerHeight - paginationRect.top + 20)
  elements.sidebar.style.top = `${topOffset}px`
  elements.sidebar.style.bottom = `${bottomOffset}px`
}

async function loadCategories() {
  const res = await fetch("/api/categories")
  state.categories = await res.json()
  renderCategories()
}

async function loadVideos(query = "") {
  const params = new URLSearchParams()
  params.set("category", state.activeCategory)
  if (query) {
    params.set("q", query)
  }
  try {
    const res = await fetch(`/api/videos?${params.toString()}`)
    if (!res.ok) {
      throw new Error(await res.text())
    }
    const data = await res.json()
    state.videos = Array.isArray(data) ? data : []
  } catch (e) {
    state.videos = []
  }
  state.currentPage = 1
  renderVideos()
}

async function loadVideoRanking() {
  try {
    const res = await fetch("/api/rankings/videos")
    if (!res.ok) return
    const data = await res.json()
    renderRankings(Array.isArray(data) ? data : [])
  } catch (_) {
    renderRankings([])
  }
}

async function loadHomepagePosters() {
  try {
    const res = await fetch("/api/homepage-posters")
    if (!res.ok) {
      throw new Error(await res.text())
    }
    const data = await res.json()
    state.posters = Array.isArray(data) ? data : []
  } catch (e) {
    state.posters = []
  }
  renderPosters()
}

async function searchVideo() {
  const raw = elements.searchInput.value.trim()
  await loadVideos(raw)
}

elements.searchBtn.addEventListener("click", searchVideo)
elements.searchInput.addEventListener("keydown", (event) => {
  if (event.key === "Enter") {
    searchVideo()
  }
})

window.addEventListener("resize", updateSidebarLayout)
// Message badge
function loadMsgBadge() {
  const token = localStorage.getItem("authToken")
  if (!token) return
  fetch("/api/messages/unread-count", { headers: { Authorization: "Bearer " + token } })
    .then(r => r.ok ? r.json() : null)
    .then(d => {
      if (!d) return
      const badge = document.getElementById("headerMsgBadge")
      if (badge && d.count > 0) {
        badge.textContent = d.count > 99 ? "99+" : d.count
        badge.style.display = "flex"
      }
    }).catch(() => {})
}

// Initialize login state check
initLoginFromUrl()
checkLoginState()
loadMsgBadge()
;(async function () {
  if (window.evwLangPromise) await window.evwLangPromise
  await loadFrontendConfig()
  await Promise.all([loadCategories(), loadVideos(), loadVideoRanking()])
  loadHomepagePosters()
})()









