// Auth & Header Logic
const userEntry = document.getElementById("userEntry");
const loginModal = document.getElementById("loginModal");
const closeModal = document.querySelector(".close-modal");
const doLoginBtn = document.getElementById("doLoginBtn");
const loginEmail = document.getElementById("loginEmail");
const loginPwd = document.getElementById("loginPwd");
const loginMsg = document.getElementById("loginMsg");
const forgotLink = document.getElementById("forgotLink");
const backToPwd = document.getElementById("backToPwd");
const codeLoginArea = document.getElementById("codeLoginArea");
const codeInput = document.getElementById("codeInput");
const sendCodeBtn = document.getElementById("sendCodeBtn");
const codeLoginBtn = document.getElementById("codeLoginBtn");
const toRegisterLink = document.getElementById("toRegisterLink");
const uploadBtn = document.getElementById("uploadBtn");
const postUploadBtn = document.getElementById("postUploadBtn");
const state = {
    posts: [],
    postCategories: [],
    activePostCategory: "all",
    postSearchQuery: "",
    postCurrentPage: 1,
    postPageSize: 10
};

async function loadFrontendConfig() {
    try {
        const res = await fetch("/api/app-config");
        if (!res.ok) return;
        const cfg = await res.json();
        const size = cfg && cfg.pagination && Number(cfg.pagination.postsPerPage);
        if (Number.isFinite(size) && size > 0) {
            state.postPageSize = size;
        }
    } catch (_) {}
}
const guestAvatarSvg = `data:image/svg+xml;utf8,${encodeURIComponent(
    "<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'><circle cx='32' cy='32' r='32' fill='%23E3E5E7'/><circle cx='32' cy='24' r='10' fill='%239499A0'/><path d='M12 54c4-10 16-14 20-14s16 4 20 14' fill='%239499A0'/></svg>"
)}`;

// Helper to get user
function getUser() {
    try {
        const userStr = localStorage.getItem("user");
        return userStr ? JSON.parse(userStr) : null;
    } catch (e) {
        return null;
    }
}

function migrateToken() {
    const legacy = localStorage.getItem("token");
    if (legacy && !localStorage.getItem("authToken")) {
        localStorage.setItem("authToken", legacy);
        localStorage.removeItem("token");
    }
}

function getToken() {
    const value = localStorage.getItem("authToken");
    if (!value || value === "null" || value === "undefined") {
        return "";
    }
    return value;
}

function authFetch(url, options = {}) {
    const headers = options.headers || {};
    const token = getToken();
    if (token) {
        headers.Authorization = `Bearer ${token}`;
    }
    return fetch(url, { ...options, headers });
}
if (typeof window !== "undefined") window.authFetch = authFetch;

function getLoginRedirectUrl(target) {
    const redirectTarget = target || (window.location.pathname + window.location.search);
    return `/?login=1&redirect=${encodeURIComponent(redirectTarget)}`;
}

function stripHtmlToText(html) {
    const container = document.createElement("div");
    container.innerHTML = html || "";
    return (container.textContent || "").trim();
}

function escapeHTML(text) {
    return (text || "").replace(/[&<>"']/g, (char) => {
        if (char === "&") return "&amp;";
        if (char === "<") return "&lt;";
        if (char === ">") return "&gt;";
        if (char === "\"") return "&quot;";
        return "&#39;";
    });
}

function buildPostPreview(html, limit = 120) {
    const text = stripHtmlToText(html);
    if (!text) return "";
    if (text.length <= limit) return text;
    return `${text.slice(0, limit)}...`;
}

const passwordGroup = loginPwd ? loginPwd.closest(".form-group") : null;

function showPasswordLogin() {
    if (passwordGroup) passwordGroup.style.display = "";
    if (doLoginBtn) doLoginBtn.style.display = "";
    if (forgotLink) forgotLink.style.display = "";
    if (codeLoginArea) codeLoginArea.style.display = "none";
    if (loginMsg) loginMsg.textContent = "";
}

function showCodeLogin() {
    if (passwordGroup) passwordGroup.style.display = "none";
    if (doLoginBtn) doLoginBtn.style.display = "none";
    if (forgotLink) forgotLink.style.display = "none";
    if (codeLoginArea) codeLoginArea.style.display = "block";
    if (loginMsg) loginMsg.textContent = "";
}

function openLoginModal() {
    if (!loginModal) return;
    loginModal.style.display = "flex";
    showPasswordLogin();
}

function updateHeader() {
    const user = getUser();
    const avatarEl = userEntry.querySelector(".avatar-placeholder");

    if (user && user.avatarUrl) {
        avatarEl.textContent = "";
        avatarEl.style.backgroundImage = `url(${user.avatarUrl})`;
        avatarEl.style.backgroundSize = "cover";
        avatarEl.style.backgroundPosition = "center";
        avatarEl.style.borderRadius = "50%";
    } else if (user) {
        avatarEl.textContent = user.nickname ? user.nickname[0].toUpperCase() : "U";
        avatarEl.style.background = "#00AEEC";
        avatarEl.style.color = "#fff";
        avatarEl.style.backgroundImage = "none";
    } else {
        avatarEl.textContent = "";
        avatarEl.style.background = "#E3E5E7";
        avatarEl.style.color = "#9499A0";
        avatarEl.style.backgroundImage = `url("${guestAvatarSvg}")`;
        avatarEl.style.backgroundSize = "70%";
        avatarEl.style.backgroundPosition = "center";
        avatarEl.style.backgroundRepeat = "no-repeat";
    }

    userEntry.onclick = () => {
        const token = getToken();
        if (token) {
            window.location.href = "/user.html";
            return;
        }
        if (loginModal) {
            openLoginModal();
            return;
        }
        window.location.href = getLoginRedirectUrl();
    };
}

if (uploadBtn) {
    uploadBtn.onclick = () => {
        const token = getToken();
        if (token) {
            window.location.href = "/creator.html";
            return;
        }
        if (loginModal) {
            openLoginModal();
            return;
        }
        window.location.href = getLoginRedirectUrl("/creator.html");
    };
}
if (postUploadBtn) {
    postUploadBtn.onclick = () => {
        const token = getToken();
        if (token) {
            window.location.href = "/post_create.html";
            return;
        }
        if (loginModal) {
            openLoginModal();
            return;
        }
        window.location.href = getLoginRedirectUrl("/post_create.html");
    };
}

if (closeModal) closeModal.onclick = () => { if (loginModal) loginModal.style.display = "none"; showPasswordLogin(); };
window.onclick = (event) => { if (loginModal && event.target == loginModal) { loginModal.style.display = "none"; showPasswordLogin(); } };

if (doLoginBtn) {
    doLoginBtn.onclick = async () => {
        const email = loginEmail.value.trim();
        const password = loginPwd.value.trim();
        if (!email || !password) {
            if (loginMsg) loginMsg.textContent = "请输入邮箱和密码";
            return;
        }
        
        doLoginBtn.textContent = "登录中...";
        doLoginBtn.disabled = true;
        
        try {
            const res = await fetch("/api/login", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ email, password })
            });
            if (res.status === 403) {
                const data = await res.json().catch(() => ({}));
                if (data.code === "banned") {
                    const remain = Number(data.remainingSeconds) || 0;
                    const d = Math.floor(remain / 86400), h = Math.floor((remain % 86400) / 3600), m = Math.floor((remain % 3600) / 60);
                    let t = "剩余：" + (d ? d + "天 " : "") + (h ? h + "小时 " : "") + m + "分钟";
                    alert("您的账号已被封禁。\n\n封号原因：" + (data.reason || "违反社区规范") + "\n解封时间：" + (data.bannedUntil || "") + "\n" + t);
                    doLoginBtn.disabled = false;
                    doLoginBtn.textContent = "密码登录";
                    return;
                }
            }
            if (!res.ok) {
                const text = await res.text();
                throw new Error(text || "登录失败");
            }
            const data = await res.json();
            if (!data.token) throw new Error("登录失败");
            localStorage.setItem("authToken", data.token);
            if (data.user) localStorage.setItem("user", JSON.stringify(data.user));
            location.reload();
        } catch (e) {
            if (loginMsg) loginMsg.textContent = e.message;
            doLoginBtn.disabled = false;
            doLoginBtn.textContent = "密码登录";
        }
    };
}

if (toRegisterLink) {
    toRegisterLink.onclick = () => {
        window.location.href = "/?login=1"; 
    };
}

if (forgotLink) {
    forgotLink.onclick = showCodeLogin;
}

if (backToPwd) {
    backToPwd.onclick = showPasswordLogin;
}

if (sendCodeBtn) {
    sendCodeBtn.onclick = async () => {
        const email = loginEmail.value.trim();
        if (!email) {
            if (loginMsg) {
                loginMsg.textContent = "请输入邮箱";
                loginMsg.className = "error-msg";
            }
            return;
        }
        if (!/^\S+@\S+\.\S+$/.test(email)) {
            if (loginMsg) {
                loginMsg.textContent = "请输入有效邮箱";
                loginMsg.className = "error-msg";
            }
            return;
        }
        sendCodeBtn.disabled = true;
        sendCodeBtn.textContent = "发送中...";
        if (loginMsg) loginMsg.textContent = "";
        try {
            const res = await fetch("/api/login-code/send", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ email })
            });
            if (!res.ok) {
                const text = await res.text();
                throw new Error(text || "发送失败");
            }
            if (loginMsg) {
                loginMsg.textContent = "验证码已发送";
                loginMsg.className = "success-msg";
            }
            let count = 60;
            const timer = setInterval(() => {
                count--;
                sendCodeBtn.textContent = `${count}s`;
                if (count <= 0) {
                    clearInterval(timer);
                    sendCodeBtn.disabled = false;
                    sendCodeBtn.textContent = "获取验证码";
                }
            }, 1000);
        } catch (e) {
            if (loginMsg) {
                loginMsg.textContent = e.message;
                loginMsg.className = "error-msg";
            }
            sendCodeBtn.disabled = false;
            sendCodeBtn.textContent = "获取验证码";
        }
    };
}

if (codeLoginBtn) {
    codeLoginBtn.onclick = async () => {
        const email = loginEmail.value.trim();
        const code = codeInput ? codeInput.value.trim() : "";
        if (!email || !code) {
            if (loginMsg) {
                loginMsg.textContent = "请输入邮箱和验证码";
                loginMsg.className = "error-msg";
            }
            return;
        }
        codeLoginBtn.disabled = true;
        codeLoginBtn.textContent = "登录中...";
        if (loginMsg) loginMsg.textContent = "";
        try {
            const res = await fetch("/api/login-code/verify", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ email, code })
            });
            if (res.status === 403) {
                const data = await res.json().catch(() => ({}));
                if (data.code === "banned") {
                    const remain = Number(data.remainingSeconds) || 0;
                    const d = Math.floor(remain / 86400), h = Math.floor((remain % 86400) / 3600), m = Math.floor((remain % 3600) / 60);
                    let t = "剩余：" + (d ? d + "天 " : "") + (h ? h + "小时 " : "") + m + "分钟";
                    alert("您的账号已被封禁。\n\n封号原因：" + (data.reason || "违反社区规范") + "\n解封时间：" + (data.bannedUntil || "") + "\n" + t);
                    codeLoginBtn.disabled = false;
                    codeLoginBtn.textContent = "验证码登录";
                    return;
                }
            }
            if (!res.ok) {
                const text = await res.text();
                throw new Error(text || "登录失败");
            }
            const data = await res.json();
            if (!data.token) throw new Error("登录失败");
            localStorage.setItem("authToken", data.token);
            if (data.user) localStorage.setItem("user", JSON.stringify(data.user));
            location.reload();
        } catch (e) {
            if (loginMsg) {
                loginMsg.textContent = e.message;
                loginMsg.className = "error-msg";
            }
            codeLoginBtn.disabled = false;
            codeLoginBtn.textContent = "验证码登录";
        }
    };
}

// Page Specific Logic
migrateToken();

// Message badge
(function() {
    const token = getToken();
    if (!token) return;
    fetch("/api/messages/unread-count", { headers: { Authorization: "Bearer " + token } })
        .then(r => r.ok ? r.json() : null)
        .then(d => {
            if (!d) return;
            const badge = document.getElementById("headerMsgBadge");
            if (badge && d.count > 0) {
                badge.textContent = d.count > 99 ? "99+" : d.count;
                badge.style.display = "flex";
            }
        }).catch(() => {});
})();

const path = window.location.pathname;

// 1. Posts List (Home)
if (path.endsWith("/posts.html") || path === "/posts.html") {
    const postList = document.getElementById("postList");
    const postRankList = document.getElementById("postRankList");
    const postCategoryTabs = document.getElementById("postCategoryTabs");
    const postPosterGrid = document.getElementById("postPosterGrid");
    const searchInput = document.getElementById("searchInput");
    const searchBtn = document.getElementById("searchBtn");
    const pagination = document.getElementById("pagination");
    const pageParams = new URLSearchParams(window.location.search);
    state.postSearchQuery = (pageParams.get("q") || "").trim();
    if (searchInput) {
        searchInput.value = state.postSearchQuery;
    }

    let postPosters = [];
    let postPosterIndex = 0;
    let postPosterTimer = null;

    function renderPostCategories() {
        if (!postCategoryTabs) return;
        postCategoryTabs.innerHTML = "";
        state.postCategories.forEach((cat) => {
            const btn = document.createElement("button");
            btn.className = `category ${state.activePostCategory === cat.id ? "active" : ""}`;
            btn.textContent = (cat.id === "all" && window.evwT) ? window.evwT("common.all") : cat.name;
            btn.addEventListener("click", () => {
                state.activePostCategory = cat.id;
                state.postCurrentPage = 1;
                renderPostCategories();
                loadPosts();
            });
            postCategoryTabs.appendChild(btn);
        });
    }

    async function loadPostCategories() {
        try {
            const res = await fetch("/api/post-categories");
            if (!res.ok) return;
            state.postCategories = await res.json();
            if (!state.postCategories.find((item) => item.id === state.activePostCategory)) {
                state.activePostCategory = "all";
            }
            renderPostCategories();
        } catch (e) {
        }
    }

    function renderPostRankings(rankingList) {
        if (!postRankList) return;
        var rankTitle = (window.evwT && window.evwT("posts.rankTitle")) || "帖子排行榜";
        var list = (rankingList !== undefined && Array.isArray(rankingList)) ? rankingList : [];
        if (list.length === 0) {
            postRankList.innerHTML = "<h3>" + rankTitle + "</h3>";
            return;
        }
        postRankList.innerHTML = "<h3>" + rankTitle + "</h3>";
        list.forEach((post, index) => {
            const item = document.createElement("div");
            item.className = "rank-item";
            const numSpan = document.createElement("span");
            numSpan.className = `rank-num ${index < 3 ? "top" : ""}`;
            numSpan.textContent = index + 1;
            const titleSpan = document.createElement("span");
            titleSpan.className = "rank-title";
            titleSpan.textContent = post.title;
            titleSpan.title = post.title;
            item.appendChild(numSpan);
            item.appendChild(titleSpan);
            item.onclick = () => {
                window.location.href = `/post_detail.html?id=${post.id}`;
            };
            postRankList.appendChild(item);
        });
    }

    function renderPosters() {
        if (!postPosterGrid) return;
        postPosterGrid.innerHTML = "";
        if (!Array.isArray(postPosters) || postPosters.length === 0) {
            postPosterGrid.style.display = "none";
            return;
        }
        postPosterGrid.style.display = "";
        postPosterIndex = 0;
        if (postPosterTimer) {
            clearInterval(postPosterTimer);
            postPosterTimer = null;
        }

        const track = document.createElement("div");
        track.className = "poster-carousel-track";

        postPosters.forEach((poster) => {
            const slide = document.createElement("div");
            slide.className = "poster-carousel-slide";
            const href = poster.linkUrl && String(poster.linkUrl).trim() ? poster.linkUrl.trim() : "javascript:void(0)";
            const link = document.createElement("a");
            link.href = href;
            if (poster.openInNewTab && href !== "javascript:void(0)") {
                link.target = "_blank";
                link.rel = "noopener noreferrer";
            }
            link.innerHTML = `<img src="${poster.imageUrl || ""}" alt="poster">`;
            slide.appendChild(link);
            track.appendChild(slide);
        });
        postPosterGrid.appendChild(track);

        const updatePosterPosition = () => {
            const innerTrack = postPosterGrid.querySelector(".poster-carousel-track");
            if (innerTrack) {
                innerTrack.style.transform = `translateX(-${postPosterIndex * 100}%)`;
            }
            postPosterGrid.querySelectorAll(".poster-carousel-dot").forEach((dot, idx) => {
                dot.classList.toggle("active", idx === postPosterIndex);
            });
        };

        const slidePoster = (dir) => {
            const len = postPosters.length;
            if (len <= 1) return;
            postPosterIndex = (postPosterIndex + dir + len) % len;
            updatePosterPosition();
        };

        const goToPoster = (idx) => {
            postPosterIndex = idx;
            updatePosterPosition();
            if (postPosterTimer) {
                clearInterval(postPosterTimer);
                postPosterTimer = setInterval(() => slidePoster(1), 5000);
            }
        };

        if (postPosters.length > 1) {
            const prevBtn = document.createElement("button");
            prevBtn.className = "poster-carousel-btn poster-prev";
            prevBtn.innerHTML = `<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>`;
            prevBtn.onclick = (e) => { e.preventDefault(); slidePoster(-1); };

            const nextBtn = document.createElement("button");
            nextBtn.className = "poster-carousel-btn poster-next";
            nextBtn.innerHTML = `<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 6 15 12 9 18"/></svg>`;
            nextBtn.onclick = (e) => { e.preventDefault(); slidePoster(1); };

            postPosterGrid.appendChild(prevBtn);
            postPosterGrid.appendChild(nextBtn);

            const dots = document.createElement("div");
            dots.className = "poster-carousel-dots";
            postPosters.forEach((_, i) => {
                const dot = document.createElement("button");
                dot.className = "poster-carousel-dot" + (i === 0 ? " active" : "");
                dot.onclick = (e) => { e.preventDefault(); goToPoster(i); };
                dots.appendChild(dot);
            });
            postPosterGrid.appendChild(dots);

            postPosterTimer = setInterval(() => slidePoster(1), 5000);
        }
    }

    async function loadPosters() {
        try {
            const res = await fetch("/api/homepage-posters");
            if (!res.ok) throw new Error(await res.text());
            const data = await res.json();
            postPosters = Array.isArray(data) ? data : [];
        } catch (e) {
            postPosters = [];
        }
        renderPosters();
    }

    function totalPostPages() {
        return Math.max(1, Math.ceil(state.posts.length / state.postPageSize));
    }

    function setPostPage(nextPage) {
        const total = totalPostPages();
        const target = Math.min(Math.max(1, nextPage), total);
        if (state.postCurrentPage === target) {
            renderPostPagination();
            return;
        }
        state.postCurrentPage = target;
        renderPostList();
    }

    function renderPostList() {
        if (!postList) return;
        postList.innerHTML = "";
        if (!state.posts || state.posts.length === 0) {
            var noPosts = (window.evwT && window.evwT("posts.noPosts")) || "暂无帖子";
            postList.innerHTML = "<div style='text-align:center;padding:40px;color:#999'>" + noPosts + "</div>";
            state.postCurrentPage = 1;
            renderPostPagination();
            renderPostRankings([]);
            return;
        }
        const total = totalPostPages();
        if (state.postCurrentPage > total) {
            state.postCurrentPage = total;
        }
        const start = (state.postCurrentPage - 1) * state.postPageSize;
        const end = start + state.postPageSize;
        const displayPosts = state.posts.slice(start, end);
        displayPosts.forEach((post) => {
            const card = document.createElement("div");
            card.className = "post-card";
            card.onclick = () => window.location.href = `/post_detail.html?id=${post.id}`;

            let imgHtml = "";
            if (post.imagePath) {
                imgHtml = `<img src="${post.imagePath}" class="post-cover" alt="cover">`;
            } else {
                imgHtml = `<div class="post-cover" style="display:flex;align-items:center;justify-content:center;color:#ccc;font-size:14px;background:#f4f5f7;">无封面</div>`;
            }

            const date = new Date(post.createdAt).toLocaleDateString();
            const avatarUrl = post.avatarUrl || "";
            const firstChar = (post.nickname || post.email || "U").charAt(0).toUpperCase();
            const avatarHtml = avatarUrl
                ? `<img src="${avatarUrl}" style="width:24px;height:24px;border-radius:50%;object-fit:cover;" alt="avatar">`
                : `<div style="width:24px;height:24px;border-radius:50%;background:#FB7299;color:#fff;font-size:12px;font-weight:bold;display:flex;align-items:center;justify-content:center;flex-shrink:0;">${escapeHTML(firstChar)}</div>`;

            const previewText = escapeHTML(buildPostPreview(post.content));
            card.innerHTML = `
                ${imgHtml}
                <div class="post-info">
                    <div>
                        <div class="post-title">${post.title}</div>
                        <div class="post-preview">${previewText || ""}</div>
                    </div>
                    <div class="post-meta">
                        <div class="post-author">
                            ${avatarHtml}
                            <span>${post.nickname || post.email}</span>
                        </div>
                        <div class="post-stats">
                            <span class="post-stat">${date}</span>
                            <span class="post-stat">赞 ${post.likeCount || 0}</span>
                            <span class="post-stat">阅 ${post.views || 0}</span>
                        </div>
                    </div>
                </div>
            `;
            postList.appendChild(card);
        });
        renderPostPagination();
    }

    async function loadPostRankings() {
        if (!postRankList) return;
        try {
            const res = await fetch("/api/rankings/posts");
            const raw = await res.json().catch(function() { return []; });
            const list = Array.isArray(raw) ? raw : [];
            renderPostRankings(res.ok ? list : []);
        } catch (e) {
            renderPostRankings([]);
        }
    }

    function renderPostPagination() {
        if (!pagination) return;
        pagination.innerHTML = "";
        const total = totalPostPages();
        var prevT = (window.evwT && window.evwT("common.pagination.prev")) || "上一页";
        var nextT = (window.evwT && window.evwT("common.pagination.next")) || "下一页";
        var jumpT = (window.evwT && window.evwT("common.pagination.jumpTo")) || "跳至";
        var pageT = (window.evwT && window.evwT("common.pagination.page")) || "页";

        const createBtn = (text, onClick, className = "", disabled = false) => {
            const btn = document.createElement("button");
            btn.className = `page-btn ${className}`.trim();
            btn.textContent = text;
            btn.disabled = disabled;
            if (onClick && !disabled) {
                btn.addEventListener("click", onClick);
            }
            return btn;
        };

        pagination.appendChild(createBtn(prevT, () => setPostPage(state.postCurrentPage - 1), "prev-btn", state.postCurrentPage === 1));

        const windowSize = 2;
        let pages = [];
        if (total <= 7) {
            for (let i = 1; i <= total; i++) pages.push(i);
        } else {
            let left = state.postCurrentPage - windowSize;
            let right = state.postCurrentPage + windowSize;
            if (left <= 2) {
                left = 1;
                right = Math.min(total, 5 + (windowSize * 2));
            }
            if (right >= total - 1) {
                right = total;
                left = Math.max(1, total - (4 + (windowSize * 2)));
            }
            if (left > 1) {
                pages.push(1);
                if (left > 2) pages.push("...");
            }
            for (let i = Math.max(1, left); i <= Math.min(total, right); i++) {
                pages.push(i);
            }
            if (right < total) {
                if (right < total - 1) pages.push("...");
                pages.push(total);
            }
        }
        pages = [...new Set(pages)];
        pages.forEach((p) => {
            if (p === "...") {
                const span = document.createElement("span");
                span.className = "page-dots";
                span.textContent = "...";
                pagination.appendChild(span);
            } else {
                pagination.appendChild(createBtn(String(p), () => setPostPage(p), p === state.postCurrentPage ? "active" : ""));
            }
        });

        pagination.appendChild(createBtn(nextT, () => setPostPage(state.postCurrentPage + 1), "next-btn", state.postCurrentPage === total));

        const jumpContainer = document.createElement("div");
        jumpContainer.className = "page-jump";
        const jumpText1 = document.createElement("span");
        jumpText1.textContent = jumpT;
        jumpContainer.appendChild(jumpText1);
        const jumpInput = document.createElement("input");
        jumpInput.type = "text";
        jumpInput.className = "jump-input";
        jumpInput.addEventListener("keydown", (e) => {
            if (e.key === "Enter") {
                const val = parseInt(jumpInput.value);
                if (!isNaN(val) && val >= 1 && val <= total) {
                    setPostPage(val);
                } else {
                    jumpInput.value = "";
                }
            }
        });
        jumpContainer.appendChild(jumpInput);
        const jumpText2 = document.createElement("span");
        jumpText2.textContent = pageT;
        jumpContainer.appendChild(jumpText2);
        pagination.appendChild(jumpContainer);
    }

    async function loadPosts() {
        try {
            const params = new URLSearchParams();
            if (state.activePostCategory) {
                params.set("category", state.activePostCategory);
            }
            if (state.postSearchQuery) {
                params.set("q", state.postSearchQuery);
            }
            const url = params.toString() ? `/api/posts?${params.toString()}` : "/api/posts";
            const res = await fetch(url);
            if (!res.ok) {
                throw new Error(await res.text() || "加载失败");
            }
            const posts = await res.json();
            state.posts = posts || [];
            renderPostList();
            loadPostRankings();
        } catch (e) {
            if (postList) postList.innerHTML = "<div style='text-align:center;padding:40px;color:red'>加载失败</div>";
            if (postRankList) postRankList.innerHTML = "<h3>" + ((window.evwT && window.evwT("posts.rankTitle")) || "帖子排行榜") + "</h3>";
            renderPostPagination();
        }
    }

    if (postList) {
        (async function() {
            if (window.evwLangPromise) await window.evwLangPromise;
            await loadFrontendConfig();
            await loadPostCategories();
            await loadPosts();
            loadPostRankings();
        })();
        loadPosters();
    }

    const applyPostSearch = () => {
        if (!searchInput) return;
        state.postSearchQuery = searchInput.value.trim();
        state.postCurrentPage = 1;
        const params = new URLSearchParams();
        if (state.activePostCategory) {
            params.set("category", state.activePostCategory);
        }
        if (state.postSearchQuery) {
            params.set("q", state.postSearchQuery);
        }
        const nextUrl = params.toString() ? `/posts.html?${params.toString()}` : "/posts.html";
        window.history.replaceState({}, "", nextUrl);
        loadPosts();
    };
    if (searchBtn) {
        searchBtn.addEventListener("click", applyPostSearch);
    }
    if (searchInput) {
        searchInput.addEventListener("keydown", (event) => {
            if (event.key === "Enter") {
                applyPostSearch();
            }
        });
    }
}

// 2. Create Post
if (path.endsWith("/post_create.html")) {
    const submitBtn = document.getElementById("submitPostBtn");
    const titleInput = document.getElementById("postTitle");
    const contentInput = document.getElementById("postContent");
    const categorySelect = document.getElementById("postCategorySelect");
    const msg = document.getElementById("postMsg");
    const postPublishQuota = document.getElementById("postPublishQuota");
    const thumbUploadBtn = document.getElementById("postThumbUploadBtn");
    const thumbPreview = document.getElementById("postThumbPreview");
    const thumbInput = document.getElementById("postThumbInput");
    let thumbUrl = "";
    
    // Check auth
    if (!getToken()) {
        if (loginModal) {
            loginModal.style.display = "flex";
        } else {
            window.location.href = getLoginRedirectUrl("/post_create.html");
        }
    }

    const loadPostCategories = async () => {
        if (!categorySelect) return;
        try {
            const res = await fetch("/api/post-categories");
            if (!res.ok) {
                categorySelect.innerHTML = "";
                return;
            }
            const list = await res.json();
            const categories = (list || []).filter((item) => item.id !== "all");
            categorySelect.innerHTML = "";
            if (categories.length === 0) {
                if (submitBtn) submitBtn.disabled = true;
                categorySelect.innerHTML = "<option value=''>暂无分类</option>";
                return;
            }
            categories.forEach((item) => {
                const option = document.createElement("option");
                option.value = item.id;
                option.textContent = item.name;
                categorySelect.appendChild(option);
            });
            if (submitBtn) submitBtn.disabled = false;
        } catch (e) {
        }
    };

    const loadPostPublishQuota = async () => {
        if (!postPublishQuota || !getToken()) return;
        try {
            const res = await authFetch("/api/creator/publish-quota");
            if (!res.ok) throw new Error("failed to load quota");
            const data = await res.json();
            const remaining = typeof data.postRemaining === "number" ? data.postRemaining : "--";
            const likesPerBonus = typeof data.likesPerBonus === "number" ? data.likesPerBonus : 20;
            var fmt = (window.evwT && window.evwT("post_create.quotaFormat")) || "剩余发布次数：{remaining}（每{likesPerBonus}赞+1次）";
            postPublishQuota.textContent = fmt.replace(/\{remaining\}/g, String(remaining)).replace(/\{likesPerBonus\}/g, String(likesPerBonus));
        } catch (e) {
            postPublishQuota.textContent = (window.evwT && window.evwT("post_create.quotaUnknown")) || "剩余发布次数：--";
        }
    };

    loadPostCategories();
    loadPostPublishQuota();

    if (thumbUploadBtn && thumbInput) {
        thumbUploadBtn.onclick = () => {
            thumbInput.click();
        };
        thumbInput.addEventListener("change", async () => {
            const file = thumbInput.files && thumbInput.files[0];
            if (!file) return;
            const formData = new FormData();
            formData.append("image", file);
            thumbUploadBtn.disabled = true;
            thumbUploadBtn.textContent = "上传中...";
            try {
                const res = await authFetch("/api/post-images", {
                    method: "POST",
                    body: formData
                });
                if (res.status === 401) {
                    localStorage.removeItem("authToken");
                    window.location.href = getLoginRedirectUrl();
                    return;
                }
                if (!res.ok) throw new Error(await res.text() || "上传失败");
                const data = await res.json();
                thumbUrl = data.url || "";
                if (thumbPreview) {
                    if (thumbUrl) {
                        thumbPreview.innerHTML = `<img src="${thumbUrl}" alt="thumb">`;
                    } else {
                        thumbPreview.textContent = "未上传";
                    }
                }
            } catch (e) {
                if (msg) msg.textContent = e.message;
            } finally {
                thumbInput.value = "";
                thumbUploadBtn.disabled = false;
                thumbUploadBtn.textContent = "上传缩略图";
            }
        });
    }

    if (submitBtn) {
        submitBtn.onclick = async () => {
            const title = titleInput.value.trim();
            const editor = window.tiptapEditor || null;
            const content = editor ? editor.getHTML() : (contentInput ? contentInput.innerHTML : "");
            const contentText = stripHtmlToText(content);
            const category = categorySelect ? categorySelect.value : "";
            if (!title || !contentText || !category) {
                if (msg) msg.textContent = "分类、标题和内容不能为空";
                return;
            }
            const contentLength = Array.from(contentText).length;
            if (contentLength > 20000) {
                if (msg) msg.textContent = "帖子正文不能超过20000字";
                return;
            }
            const imageCount = (content.match(/<img\b/gi) || []).length + (thumbUrl ? 1 : 0);
            if (imageCount > 25) {
                if (msg) msg.textContent = "帖子图片不能超过25张";
                return;
            }
            
            const formData = new FormData();
            formData.append("title", title);
            formData.append("content", content);
            formData.append("category", category);
            if (thumbUrl) {
                formData.append("imagePath", thumbUrl);
            }
            submitBtn.disabled = true;
            submitBtn.textContent = "发布中...";
            
            try {
                const res = await authFetch("/api/posts", {
                    method: "POST",
                    body: formData
                });
                
                if (res.status === 401) {
                    localStorage.removeItem("authToken");
                    window.location.href = getLoginRedirectUrl();
                    return;
                }
                if (!res.ok) throw new Error(await res.text() || "发布失败");
                
                const data = await res.json();
                window.location.href = `/post_detail.html?id=${data.id}`;
            } catch (e) {
                if (msg) msg.textContent = e.message;
                submitBtn.disabled = false;
                submitBtn.textContent = "发布";
            }
        };
    }
}

// 3. Post Detail
if (path.endsWith("/post_detail.html")) {
    const detailContainer = document.getElementById("postDetail");
    const params = new URLSearchParams(window.location.search);
    const id = params.get("id");
    const adminReviewToken = params.get("adminReviewToken") || "";
    const elements = {
        commentList: document.getElementById("commentList"),
        commentCountHeader: document.getElementById("commentCountHeader"),
        commentSortHot: document.getElementById("commentSortHot"),
        commentSortNew: document.getElementById("commentSortNew"),
        commentSubmit: document.getElementById("commentSubmit"),
        commentInput: document.getElementById("commentInput"),
        commentMsg: document.getElementById("commentMsg"),
        replyHint: document.getElementById("replyHint"),
        replyText: document.getElementById("replyText"),
        replyCancel: document.getElementById("replyCancel"),
        myCommentAvatar: document.getElementById("myCommentAvatar"),
        uploaderAvatar: document.getElementById("uploaderAvatar"),
        uploaderName: document.getElementById("uploaderName"),
        uploaderDesc: document.getElementById("uploaderDesc"),
        recListContainer: document.getElementById("recListContainer")
    };
    let commentSort = "hot";
    let commentCache = [];
    let replyTarget = null;
    let replyVisibleCount = new Map();
    let currentUserEmail = "";
    let isPostLiked = false;
    let isPostFavorited = false;
    let hasRecordedPostView = false;
    let postLikeBtn = null;
    let postLikeCountEl = null;
    let postViewCountEl = null;
    let postFavBtn = null;
    let postFavTextEl = null;
    
    function setCommentMsg(text, type = "") {
        if (!elements.commentMsg) return;
        elements.commentMsg.textContent = text;
        elements.commentMsg.className = `msg-box ${type}`;
        elements.commentMsg.classList.remove("hidden");
        if (type === "success") {
            setTimeout(() => elements.commentMsg.classList.add("hidden"), 3000);
        }
    }
    
    function updateCommentSortUI() {
        if (elements.commentSortHot) {
            elements.commentSortHot.classList.toggle("active", commentSort === "hot");
        }
        if (elements.commentSortNew) {
            elements.commentSortNew.classList.toggle("active", commentSort === "new");
        }
    }
    
    function sortReplies(list) {
        return [...list].sort((a, b) => {
            const likeDiff = (b.likeCount || 0) - (a.likeCount || 0);
            if (commentSort === "hot") {
                if (likeDiff !== 0) return likeDiff;
            }
            const timeA = a.createdAt ? new Date(a.createdAt).getTime() : 0;
            const timeB = b.createdAt ? new Date(b.createdAt).getTime() : 0;
            return timeB - timeA;
        });
    }
    
    function renderComments(comments) {
        if (!elements.commentList) return;
        elements.commentList.innerHTML = "";
        if (elements.commentCountHeader) {
            elements.commentCountHeader.textContent = `(${comments ? comments.length : 0})`;
        }
        if (!comments || comments.length === 0) {
            const empty = document.createElement("div");
            empty.style.textAlign = "center";
            empty.style.color = "#999";
            empty.style.padding = "40px 0";
            empty.textContent = "暂无评论，快来抢沙发吧";
            elements.commentList.appendChild(empty);
            return;
        }
        const commentMap = new Map();
        comments.forEach((item) => commentMap.set(item.id, item));
        const topComments = comments.filter((item) => !item.parentId);
        const repliesByRoot = new Map();
        const findRootId = (item) => {
            let parentId = item.parentId;
            let guard = 0;
            while (parentId && commentMap.has(parentId) && guard < 20) {
                const parent = commentMap.get(parentId);
                if (!parent.parentId) return parent.id;
                parentId = parent.parentId;
                guard += 1;
            }
            return item.parentId || item.id;
        };
        comments.forEach((item) => {
            if (!item.parentId) return;
            const rootId = findRootId(item);
            if (!repliesByRoot.has(rootId)) {
                repliesByRoot.set(rootId, []);
            }
            repliesByRoot.get(rootId).push(item);
        });
        if (commentSort === "hot") {
            topComments.sort((a, b) => {
                const likeDiff = (b.likeCount || 0) - (a.likeCount || 0);
                if (likeDiff !== 0) return likeDiff;
                const replyDiff = (repliesByRoot.get(b.id)?.length || 0) - (repliesByRoot.get(a.id)?.length || 0);
                if (replyDiff !== 0) return replyDiff;
                const timeA = a.createdAt ? new Date(a.createdAt).getTime() : 0;
                const timeB = b.createdAt ? new Date(b.createdAt).getTime() : 0;
                return timeB - timeA;
            });
        } else {
            topComments.sort((a, b) => {
                const timeA = a.createdAt ? new Date(a.createdAt).getTime() : 0;
                const timeB = b.createdAt ? new Date(b.createdAt).getTime() : 0;
                return timeB - timeA;
            });
        }
        topComments.forEach((item) => {
            const card = document.createElement("div");
            card.className = "comment-item";
            card.dataset.commentId = item.id;
            const date = item.createdAt ? new Date(item.createdAt).toLocaleString() : "";
            const firstChar = (item.nickname || "U").charAt(0).toUpperCase();
            const avatarStyle = item.avatarUrl
                ? `background-image: url('${item.avatarUrl}'); background-color: transparent;`
                : "background-image: none; background-color: #E3E5E7; display:flex; align-items:center; justify-content:center; color:#999; font-weight:bold;";
            const avatarContent = item.avatarUrl ? "" : firstChar;
            const replies = sortReplies(repliesByRoot.get(item.id) || []);
            const visibleCount = Math.min(replies.length, replyVisibleCount.get(item.id) || 3);
            const replyListHtml = replies.slice(0, visibleCount).map((reply) => {
                const replyDate = reply.createdAt ? new Date(reply.createdAt).toLocaleString() : "";
                const replyFirstChar = (reply.nickname || "U").charAt(0).toUpperCase();
                const replyAvatarStyle = reply.avatarUrl
                    ? `background-image: url('${reply.avatarUrl}'); background-color: transparent;`
                    : "background-image: none; background-color: #E3E5E7; display:flex; align-items:center; justify-content:center; color:#999; font-weight:bold;";
                const replyAvatarContent = reply.avatarUrl ? "" : replyFirstChar;
                const replyDelete = reply.email && currentUserEmail && reply.email === currentUserEmail
                    ? `<span class="c-action comment-delete" data-id="${reply.id}">删除</span>`
                    : "";
                const replyPendingBadge = (reply.reviewStatus === "pending" && reply.email === currentUserEmail)
                    ? '<span class="comment-pending-badge" style="margin-left:4px;font-size:11px;color:#f57c00;">审核中</span>'
                    : "";
                return `
                    <div class="reply-item" data-comment-id="${reply.id}">
                        <div class="reply-avatar avatar-link" data-email="${reply.email || ""}" style="${replyAvatarStyle}">${replyAvatarContent}</div>
                        <div class="reply-body">
                            <div class="reply-user">
                                <span class="user-link" data-email="${reply.email || ""}">${reply.nickname}${replyPendingBadge}</span>
                                <span class="reply-to">回复 @${reply.parentNickname || "未知"}</span>
                            </div>
                            <div class="reply-text">${reply.content}</div>
                            <div class="reply-info">
                                <span>${replyDate}</span>
                                <span class="c-action like-btn ${reply.liked ? "active" : ""}" data-id="${reply.id}">
                                    <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><path d="M1 21h4V9H1v12zm22-11c0-1.1-.9-2-2-2h-6.31l.95-4.57.03-.32c0-.41-.17-.79-.44-1.06L14.17 1 7.59 7.59C7.22 7.95 7 8.45 7 9v10c0 1.1.9 2 2 2h9c.83 0 1.54-.5 1.84-1.22l3.02-7.05c.09-.23.14-.47.14-.73v-1.91l-.01-.01L23 10z"></path></svg>
                                    <span class="like-num">${reply.likeCount || 0}</span>
                                </span>
                                <span class="c-action reply-btn" data-id="${reply.id}" data-nickname="${reply.nickname}">回复</span>
                                ${replyDelete}
                            </div>
                        </div>
                    </div>
                `;
            }).join("");
            const remaining = replies.length - visibleCount;
            const moreHtml = remaining > 0 ? `<div class="reply-more" data-id="${item.id}">继续展开（还剩 ${remaining} 条）</div>` : "";
            const deleteHtml = item.email && currentUserEmail && item.email === currentUserEmail
                ? `<span class="c-action comment-delete" data-id="${item.id}">删除</span>`
                : "";
            const pendingBadge = (item.reviewStatus === "pending" && item.email === currentUserEmail)
                ? '<span class="comment-pending-badge" style="margin-left:6px;font-size:12px;color:#f57c00;">审核中，仅自己可见</span>'
                : "";
            card.innerHTML = `
               <div class="c-avatar avatar-link" data-email="${item.email || ""}" style="${avatarStyle}">${avatarContent}</div>
               <div class="c-content-wrap">
                   <div class="c-user user-link" data-email="${item.email || ""}">${item.nickname}${pendingBadge}</div>
                   <div class="c-text">${item.content}</div>
                   <div class="c-info">
                       <span>${date}</span>
                       <span class="c-action like-btn ${item.liked ? "active" : ""}" data-id="${item.id}">
                           <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><path d="M1 21h4V9H1v12zm22-11c0-1.1-.9-2-2-2h-6.31l.95-4.57.03-.32c0-.41-.17-.79-.44-1.06L14.17 1 7.59 7.59C7.22 7.95 7 8.45 7 9v10c0 1.1.9 2 2 2h9c.83 0 1.54-.5 1.84-1.22l3.02-7.05c.09-.23.14-.47.14-.73v-1.91l-.01-.01L23 10z"></path></svg>
                           <span class="like-num">${item.likeCount || 0}</span>
                       </span>
                       <span class="c-action reply-btn" data-id="${item.id}" data-nickname="${item.nickname}">回复</span>
                       ${deleteHtml}
                   </div>
                   <div class="reply-list">
                      ${replyListHtml}
                   </div>
                   ${moreHtml}
               </div>
            `;
            card.querySelectorAll(".like-btn").forEach((btn) => {
                const likeNum = btn.querySelector(".like-num");
                btn.addEventListener("click", () => toggleCommentLike(Number(btn.dataset.id), btn, likeNum));
            });
            card.querySelectorAll(".reply-btn").forEach((btn) => {
                btn.addEventListener("click", () => {
                    const token = getToken();
                    if (!token) {
                        window.location.href = getLoginRedirectUrl();
                        return;
                    }
                    setReplyTarget({ id: Number(btn.dataset.id), nickname: btn.dataset.nickname });
                });
            });
            card.querySelectorAll(".avatar-link, .user-link").forEach((node) => {
                node.addEventListener("click", () => {
                    const email = node.dataset.email;
                    if (email) {
                        window.location.href = `/user.html?email=${encodeURIComponent(email)}`;
                    }
                });
            });
            card.querySelectorAll(".comment-delete").forEach((btn) => {
                btn.addEventListener("click", () => deleteComment(btn.dataset.id));
            });
            const moreBtn = card.querySelector(".reply-more");
            if (moreBtn) {
                moreBtn.addEventListener("click", () => {
                    const current = replyVisibleCount.get(item.id) || 3;
                    replyVisibleCount.set(item.id, current + 3);
                    renderComments(commentCache);
                });
            }
            elements.commentList.appendChild(card);
        });
    }
    
    function setReplyTarget(target) {
        replyTarget = target;
        if (!elements.replyHint || !elements.replyText) return;
        if (!replyTarget) {
            elements.replyHint.classList.add("hidden");
            elements.replyText.textContent = "";
            return;
        }
        elements.replyText.textContent = `回复 @${replyTarget.nickname}`;
        elements.replyHint.classList.remove("hidden");
    }
    
    function scrollToCommentFromHash() {
        const hash = window.location.hash;
        if (hash && hash.startsWith("#comment-")) {
            const commentId = hash.replace("#comment-", "");
            const tryScroll = (retries) => {
                const el = document.querySelector(`[data-comment-id="${commentId}"]`);
                if (el) {
                    const commentListEl = document.getElementById("commentList");
                    if (commentListEl) commentListEl.style.maxHeight = "none";
                    setTimeout(() => {
                        el.scrollIntoView({ behavior: "smooth", block: "center" });
                        el.style.transition = "background 0.3s";
                        el.style.background = "#fff8e1";
                        setTimeout(() => { el.style.background = ""; }, 3000);
                    }, 200);
                } else if (retries > 0) {
                    setTimeout(() => tryScroll(retries - 1), 500);
                }
            };
            tryScroll(10);
        }
    }

    async function loadComments() {
        try {
            const res = await authFetch(`/api/posts/${encodeURIComponent(id)}/comments`);
            if (!res.ok) {
                renderComments([]);
                return;
            }
            const list = await res.json();
            commentCache = list || [];
            replyVisibleCount = new Map();
            updateCommentSortUI();
            renderComments(list);
            scrollToCommentFromHash();
        } catch (e) {
            renderComments([]);
        }
    }
    
    async function toggleCommentLike(commentId, btn, numSpan) {
        const token = getToken();
        if (!token) {
            window.location.href = getLoginRedirectUrl();
            return;
        }
        try {
            const res = await authFetch(`/api/post-comments/${commentId}/like`, { method: "POST" });
            if (res.status === 401) {
                localStorage.removeItem("authToken");
                window.location.href = getLoginRedirectUrl("/post_create.html");
                return;
            }
            if (!res.ok) throw new Error();
            const data = await res.json();
            if (data.liked) btn.classList.add("active");
            else btn.classList.remove("active");
            numSpan.textContent = data.likeCount;
        } catch (e) {
        }
    }
    
    async function deleteComment(commentId) {
        const token = getToken();
        if (!token) {
            window.location.href = getLoginRedirectUrl();
            return;
        }
        if (!confirm("确定删除这条评论吗？")) return;
        try {
            const res = await authFetch(`/api/post-comments/${commentId}`, { method: "DELETE" });
            if (res.status === 401) {
                localStorage.removeItem("authToken");
                window.location.href = getLoginRedirectUrl();
                return;
            }
            if (!res.ok) throw new Error(await res.text());
            await loadComments();
        } catch (e) {
            alert("删除失败");
        }
    }
    
    async function loadMyAvatar() {
        const token = getToken();
        if (!token || !elements.myCommentAvatar) return;
        try {
            const res = await authFetch("/api/profile");
            if (res.ok) {
                const user = await res.json();
                currentUserEmail = user.email || "";
                if (user.avatarUrl) {
                    elements.myCommentAvatar.style.backgroundImage = `url('${user.avatarUrl}')`;
                    elements.myCommentAvatar.style.backgroundColor = "transparent";
                    elements.myCommentAvatar.innerHTML = "";
                } else {
                    const firstChar = (user.nickname || "U").charAt(0).toUpperCase();
                    elements.myCommentAvatar.style.backgroundImage = "none";
                    elements.myCommentAvatar.style.backgroundColor = "#E3E5E7";
                    elements.myCommentAvatar.innerHTML = `<span style="display:flex; width:100%; height:100%; align-items:center; justify-content:center; color:#999; font-weight:bold;">${firstChar}</span>`;
                }
            }
        } catch (e) {
        }
    }

    function openUserPage(email) {
        if (!email) return;
        if (currentUserEmail && email === currentUserEmail) {
            window.location.href = "/user.html";
            return;
        }
        window.location.href = `/user.html?email=${encodeURIComponent(email)}`;
    }

    function updatePostLikeButton() {
        if (!postLikeBtn) return;
        postLikeBtn.classList.toggle("active", isPostLiked);
    }

    function updatePostFavoriteButton() {
        if (!postFavBtn || !postFavTextEl) return;
        postFavBtn.classList.toggle("active", isPostFavorited);
        postFavTextEl.textContent = isPostFavorited ? "已收藏" : "收藏";
    }

    async function recordPostView(postId) {
        const token = getToken();
        if (!token || !postId || hasRecordedPostView) return;
        hasRecordedPostView = true;
        try {
            const res = await authFetch("/api/posts/view", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ postId: Number(postId) })
            });
            if (res.status === 401) {
                localStorage.removeItem("authToken");
                return;
            }
            if (!res.ok) return;
            const data = await res.json();
            if (postViewCountEl && typeof data.viewCount === "number") {
                postViewCountEl.textContent = data.viewCount;
            }
        } catch (e) {
        }
    }

    async function togglePostLike(postId) {
        const token = getToken();
        if (!token) {
            window.location.href = getLoginRedirectUrl();
            return;
        }
        if (!postId) return;
        try {
            const res = await authFetch("/api/posts/like", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ postId: Number(postId) })
            });
            if (res.status === 401) {
                localStorage.removeItem("authToken");
                window.location.href = getLoginRedirectUrl();
                return;
            }
            if (!res.ok) return;
            const data = await res.json();
            isPostLiked = !!data.isLiked;
            updatePostLikeButton();
            if (postLikeCountEl && typeof data.likeCount === "number") {
                postLikeCountEl.textContent = data.likeCount;
            }
        } catch (e) {
        }
    }

    async function togglePostFavorite(postId) {
        const token = getToken();
        if (!token) {
            window.location.href = getLoginRedirectUrl();
            return;
        }
        if (!postId) return;
        try {
            const res = await authFetch("/api/posts/favorite", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ postId: Number(postId) })
            });
            if (res.status === 401) {
                localStorage.removeItem("authToken");
                window.location.href = getLoginRedirectUrl();
                return;
            }
            if (!res.ok) return;
            const data = await res.json();
            isPostFavorited = !!data.isFavorite;
            updatePostFavoriteButton();
        } catch (e) {
        }
    }

    async function loadRecommendations(postId, authorEmail) {
        if (!elements.recListContainer) return;
        if (!authorEmail) return; // 仅展示「作者还制作过的帖子」，无作者则不请求
        try {
            const res = await fetch("/api/posts?author=" + encodeURIComponent(authorEmail));
            if (!res.ok) return;
            let list = await res.json();
            list = (list || []).filter((item) => String(item.id) !== String(postId));
            list = list.slice(0, 5);
            elements.recListContainer.innerHTML = "";
            list.forEach((post) => {
                const item = document.createElement("div");
                item.className = "rec-item";
                const hasImage = !!post.imagePath;
                const thumbStyle = hasImage ? `background-image: url('${escapeHTML(post.imagePath)}')` : "";
                const thumbClass = hasImage ? "rec-thumb" : "rec-thumb text-icon";
                const thumbContent = hasImage ? "" : "\u5E16";
                item.innerHTML = `
                    <div class="${thumbClass}" style="${thumbStyle}">${thumbContent}</div>
                    <div class="rec-info">
                        <div class="rec-title">${escapeHTML(post.title || "")}</div>
                        <div style="font-size:12px;color:#999;">${escapeHTML(post.nickname || post.email || "\u7528\u6237")}</div>
                    </div>
                `;
                item.onclick = () => {
                    window.location.href = `/post_detail.html?id=${post.id}`;
                };
                elements.recListContainer.appendChild(item);
            });
        } catch (e) {
        }
    }
    
    if (!id) {
        if (detailContainer) detailContainer.innerHTML = "参数错误";
    } else {
        async function loadDetail() {
            try {
                let postUrl = `/api/posts/${id}`;
                if (adminReviewToken) postUrl += `?adminReviewToken=${encodeURIComponent(adminReviewToken)}`;
                const res = await authFetch(postUrl);
                if (!res.ok) throw new Error("帖子不存在");
                const post = await res.json();
                
                const date = new Date(post.createdAt).toLocaleString();
                const user = getUser();
                const isOwner = user && user.email === post.email;
                const isRejected = post.reviewStatus && String(post.reviewStatus).startsWith("rejected");
                const isTakedownByAdmin = post.reviewStatus === "takedown";
                const canRequestReReview = isRejected || isTakedownByAdmin;
                
                const pd = (window.evwLang && window.evwLang.post_detail) || {};
                const getPd = (k, fallback) => (pd[k] != null ? pd[k] : fallback);
                let deleteBtn = "";
                let reReviewBtn = "";
                if (isOwner) {
                    deleteBtn = `<span class="delete-btn" id="deleteBtn">${escapeHTML(getPd("deletePost", "删除"))}</span>`;
                    if (canRequestReReview) {
                        reReviewBtn = `<span id="reReviewBtn" class="delete-btn" style="margin-left:10px;background:transparent;border:none;color:#00aeec;padding:0;cursor:pointer;">${escapeHTML(getPd("reReview", "复审"))}</span>`;
                    }
                }
                
                const avatarUrl = post.avatarUrl || "";
                const firstChar = (post.nickname || post.email || "U").charAt(0).toUpperCase();
                const avatarHtml = avatarUrl
                    ? `<img src="${(post.avatarUrl || "").replace(/'/g, "%27")}" style="width:32px;height:32px;border-radius:50%;object-fit:cover;display:block;" alt="avatar">`
                    : `<div style="width:32px;height:32px;border-radius:50%;background:#FB7299;color:#fff;font-size:16px;font-weight:bold;display:flex;align-items:center;justify-content:center;flex-shrink:0;">${escapeHTML(firstChar)}</div>`;

                const takedownBanner = (post.reviewStatus === "takedown" && post.takedownReason)
                    ? `<div class="post-takedown-banner" style="background:#e53e3e;color:#fff;padding:10px 16px;border-radius:6px;margin-bottom:12px;font-size:14px;">此帖子已下架。原因：${escapeHTML(post.takedownReason || "违反社区规范")}</div>`
                    : "";

                detailContainer.innerHTML = `
                    ${takedownBanner}
                    <div class="post-header">
                        <div class="post-title">${post.title}</div>
                        <div class="post-meta">
                            <div class="post-author" id="postAuthor">
                                ${avatarHtml}
                                <span>${post.nickname || post.email}</span>
                            </div>
                            <span>${date}</span>
                            ${deleteBtn}
                            ${reReviewBtn}
                        </div>
                        <div class="post-data">
                            <div class="post-data-item">
                                <svg viewBox="0 0 24 24"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-6h2v6zm0-8h-2V7h2v2z"></path></svg>
                                <span id="postViewCount">${post.views || 0}</span>浏览
                            </div>
                        </div>
                    </div>
                    <div class="toolbar">
                        <button class="tool-btn" id="postLikeBtn">
                            <svg viewBox="0 0 24 24"><path d="M1 21h4V9H1v12zm22-11c0-1.1-.9-2-2-2h-6.31l.95-4.57.03-.32c0-.41-.17-.79-.44-1.06L14.17 1 7.59 7.59C7.22 7.95 7 8.45 7 9v10c0 1.1.9 2 2 2h9c.83 0 1.54-.5 1.84-1.22l3.02-7.05c.09-.23.14-.47.14-.73v-1.91l-.01-.01L23 10z"></path></svg>
                            <span id="postLikeCount">${post.likeCount || 0}</span>
                        </button>
                        <button class="tool-btn" id="postFavBtn">
                            <svg viewBox="0 0 24 24"><path d="M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z"></path></svg>
                            <span id="postFavText">收藏</span>
                        </button>
                    </div>
                    <div class="post-content-wrap">
                        <div class="post-content">${post.content}</div>
                    </div>
                `;
                
                const authorNode = document.getElementById("postAuthor");
                if (authorNode) {
                    authorNode.addEventListener("click", () => openUserPage(post.email));
                }
                postLikeBtn = document.getElementById("postLikeBtn");
                postLikeCountEl = document.getElementById("postLikeCount");
                postViewCountEl = document.getElementById("postViewCount");
                postFavBtn = document.getElementById("postFavBtn");
                postFavTextEl = document.getElementById("postFavText");
                isPostLiked = !!post.isLiked;
                isPostFavorited = !!post.isFavorited;
                updatePostLikeButton();
                updatePostFavoriteButton();
                if (postLikeBtn) {
                    postLikeBtn.addEventListener("click", () => togglePostLike(id));
                }
                if (postFavBtn) {
                    postFavBtn.addEventListener("click", () => togglePostFavorite(id));
                }
                recordPostView(id);
                loadRecommendations(id, post.email);
                if (elements.uploaderName) {
                    elements.uploaderName.textContent = post.nickname || post.email || "用户";
                    elements.uploaderName.onclick = () => openUserPage(post.email);
                }
                if (elements.uploaderDesc) {
                    elements.uploaderDesc.textContent = post.authorMotto || "UP主很懒，什么都没有留下";
                }
                if (elements.uploaderAvatar) {
                    elements.uploaderAvatar.style.backgroundSize = "cover";
                    elements.uploaderAvatar.style.backgroundPosition = "center";
                    if (post.avatarUrl) {
                        elements.uploaderAvatar.style.backgroundImage = `url('${post.avatarUrl.replace(/'/g, "%27")}')`;
                        elements.uploaderAvatar.style.backgroundColor = "transparent";
                        elements.uploaderAvatar.innerHTML = "";
                    } else {
                        const firstChar = (post.nickname || "U").charAt(0).toUpperCase();
                        elements.uploaderAvatar.style.backgroundImage = "none";
                        elements.uploaderAvatar.style.backgroundColor = "#FB7299";
                        elements.uploaderAvatar.innerHTML = `<span style="position:absolute; top:50%; left:50%; transform:translate(-50%, -50%); color:#fff; font-size:20px; font-weight:bold;">${escapeHTML(firstChar)}</span>`;
                    }
                }
                
                if (isOwner) {
                    const btn = document.getElementById("deleteBtn");
                    if (btn) {
                        btn.onclick = async () => {
                            if (!confirm(getPd("confirmDelete", "确定删除吗？"))) return;
                            try {
                                const delRes = await authFetch(`/api/posts/${id}`, { method: "DELETE" });
                                if (delRes.status === 401) {
                                    localStorage.removeItem("authToken");
                                    window.location.href = getLoginRedirectUrl();
                                    return;
                                }
                                if (delRes.ok) {
                                    window.location.href = "/posts.html";
                                } else {
                                    alert(await delRes.text() || getPd("deleteFail", "删除失败"));
                                }
                            } catch (e) {
                                alert(getPd("deleteFail", "删除失败"));
                            }
                        };
                    }
                    const reReviewBtnEl = document.getElementById("reReviewBtn");
                    if (reReviewBtnEl) {
                        reReviewBtnEl.onclick = async () => {
                            if (!confirm(getPd("confirmReReview", "确认提交到管理员人工复审吗？"))) return;
                            try {
                                const res = await authFetch(`/api/posts/${id}/manual-review`, { method: "POST" });
                                const data = await res.json().catch(() => ({}));
                                if (!res.ok) {
                                    alert(data.error || getPd("submitReReviewFail", "提交复审失败"));
                                    return;
                                }
                                if (data.status === "already_pending") {
                                    alert(getPd("alreadyPending", "该帖子已有待处理复审请求，请等待管理员处理"));
                                } else {
                                    alert(getPd("reReviewSubmitted", "复审请求已提交，结果会通过消息通知"));
                                }
                            } catch (e) {
                                alert(getPd("submitReReviewFail", "提交复审失败"));
                            }
                        };
                    }
                }
                
            } catch (e) {
                if (detailContainer) detailContainer.innerHTML = `<div style="text-align:center;padding:40px;">${e.message}</div>`;
            }
        }
        if (detailContainer) loadDetail();
    }
    if (elements.commentSortHot) {
        elements.commentSortHot.addEventListener("click", () => {
            commentSort = "hot";
            updateCommentSortUI();
            renderComments(commentCache);
        });
    }
    if (elements.commentSortNew) {
        elements.commentSortNew.addEventListener("click", () => {
            commentSort = "new";
            updateCommentSortUI();
            renderComments(commentCache);
        });
    }
    if (elements.commentSubmit) {
        elements.commentSubmit.addEventListener("click", async () => {
            const content = elements.commentInput ? elements.commentInput.value.trim() : "";
            if (!content) {
                setCommentMsg("请输入内容", "error");
                return;
            }
            const token = getToken();
            if (!token) {
                window.location.href = getLoginRedirectUrl();
                return;
            }
            try {
                elements.commentSubmit.disabled = true;
                const res = await authFetch(`/api/posts/${encodeURIComponent(id)}/comments`, {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({ content, parentId: replyTarget ? replyTarget.id : 0 })
                });
                if (res.status === 401) {
                    localStorage.removeItem("authToken");
                    window.location.href = getLoginRedirectUrl();
                    return;
                }
                if (!res.ok) throw new Error(await res.text());
                if (elements.commentInput) elements.commentInput.value = "";
                setReplyTarget(null);
                setCommentMsg("评论已提交", "success");
                await loadComments();
            } catch (e) {
                setCommentMsg(e.message || "发布失败", "error");
            } finally {
                elements.commentSubmit.disabled = false;
            }
        });
    }
    if (elements.replyCancel) {
        elements.replyCancel.addEventListener("click", () => setReplyTarget(null));
    }
    loadComments();
    loadMyAvatar();
}

// Init Header
updateHeader();






