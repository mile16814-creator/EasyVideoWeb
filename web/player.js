
const elements = {
  playerVideo: document.getElementById("playerVideo"),
  playerFallback: document.getElementById("playerFallback"),
  playerTitle: document.getElementById("playerTitle"),
  viewCount: document.getElementById("viewCount"),
  publishDate: document.getElementById("publishDate"),
  deleteBtn: document.getElementById("deleteBtn"),
  reReviewBtn: document.getElementById("reReviewBtn"),
  reportVideoBtn: document.getElementById("reportVideoBtn"),
  
  btnLike: document.getElementById("btnLike"),
  likeCount: document.getElementById("likeCount"),
  btnFav: document.getElementById("btnFav"),
  favText: document.getElementById("favText"),
  
  playerDesc: document.getElementById("playerDesc"),
  videoTags: document.getElementById("videoTags"),
  
  uploaderAvatar: document.getElementById("uploaderAvatar"),
  uploaderName: document.getElementById("uploaderName"),
  uploaderDesc: document.getElementById("uploaderDesc"),
  
  recListContainer: document.getElementById("recListContainer"),
  
  commentCountHeader: document.getElementById("commentCountHeader"),
  myCommentAvatar: document.getElementById("myCommentAvatar"),
  commentInput: document.getElementById("commentInput"),
  commentSubmit: document.getElementById("commentSubmit"),
  commentMsg: document.getElementById("commentMsg"),
  commentList: document.getElementById("commentList"),
  replyHint: document.getElementById("replyHint"),
  replyText: document.getElementById("replyText"),
  replyCancel: document.getElementById("replyCancel"),
  commentSortHot: document.getElementById("commentSortHot"),
  commentSortNew: document.getElementById("commentSortNew"),
  headerUserEntry: document.getElementById("userEntry"),
  headerAvatar: document.querySelector("#userEntry .avatar-placeholder"),
  headerUploadBtn: document.getElementById("uploadBtn"),
  headerPostUploadBtn: document.getElementById("postUploadBtn")
};

const getToken = () => {
  const value = localStorage.getItem("authToken");
  if (!value || value === "null" || value === "undefined") {
    return "";
  }
  return value;
};

const authFetch = (url, options = {}) => {
  const headers = options.headers || {};
  const token = getToken();
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }
  return fetch(url, { ...options, headers });
};

const getLoginRedirectUrl = (target) => {
  const redirectTarget = target || (window.location.pathname + window.location.search);
  return `/?login=1&redirect=${encodeURIComponent(redirectTarget)}`;
};

const guestAvatarSvg = `data:image/svg+xml;utf8,${encodeURIComponent(
  "<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'><circle cx='32' cy='32' r='32' fill='%23E3E5E7'/><circle cx='32' cy='24' r='10' fill='%239499A0'/><path d='M12 54c4-10 16-14 20-14s16 4 20 14' fill='%239499A0'/></svg>"
)}`;

function applyHeaderAvatar(user) {
  if (!elements.headerAvatar) return;
  const avatarEl = elements.headerAvatar;
  if (user && user.avatarUrl) {
    avatarEl.textContent = "";
    avatarEl.style.backgroundImage = `url(${user.avatarUrl})`;
    avatarEl.style.backgroundSize = "cover";
    avatarEl.style.backgroundPosition = "center";
    avatarEl.style.backgroundRepeat = "no-repeat";
    avatarEl.style.backgroundColor = "transparent";
  } else {
    const nickname = user && user.nickname ? user.nickname : "U";
    avatarEl.textContent = nickname ? nickname[0].toUpperCase() : "U";
    avatarEl.style.background = "#00AEEC";
    avatarEl.style.color = "#fff";
    avatarEl.style.backgroundImage = "none";
  }
}

function applyHeaderLoggedOut() {
  if (!elements.headerAvatar) return;
  const avatarEl = elements.headerAvatar;
  avatarEl.textContent = "";
  avatarEl.style.background = "#E3E5E7";
  avatarEl.style.color = "#9499A0";
  avatarEl.style.backgroundImage = `url("${guestAvatarSvg}")`;
  avatarEl.style.backgroundSize = "70%";
  avatarEl.style.backgroundPosition = "center";
  avatarEl.style.backgroundRepeat = "no-repeat";
}

function initHeader() {
  const token = getToken();
  if (elements.headerUserEntry) {
    elements.headerUserEntry.addEventListener("click", () => {
      if (token) {
        window.location.href = "/user.html";
        return;
      }
      window.location.href = getLoginRedirectUrl();
    });
  }
  if (elements.headerUploadBtn) {
    elements.headerUploadBtn.addEventListener("click", () => {
      if (token) {
        window.location.href = "/creator.html";
        return;
      }
      window.location.href = getLoginRedirectUrl("/creator.html");
    });
  }
  if (elements.headerPostUploadBtn) {
    elements.headerPostUploadBtn.addEventListener("click", () => {
      if (token) {
        window.location.href = "/post_create.html";
        return;
      }
      window.location.href = getLoginRedirectUrl("/post_create.html");
    });
  }
  if (!token) {
    applyHeaderLoggedOut();
    return;
  }
  const userStr = localStorage.getItem("user");
  if (userStr) {
    try {
      applyHeaderAvatar(JSON.parse(userStr));
      return;
    } catch (e) {
      localStorage.removeItem("user");
    }
  }
  applyHeaderAvatar({ nickname: "U" });
}

function formatDuration(seconds) {
  if (!seconds) return "-";
  const total = Math.floor(seconds);
  const m = String(Math.floor(total / 60)).padStart(2, "0");
  const s = String(total % 60).padStart(2, "0");
  return `${m}:${s}`;
}

function formatDate(isoString) {
  if (!isoString) return "-";
  const date = new Date(isoString);
  return `${date.getFullYear()}-${String(date.getMonth()+1).padStart(2,'0')}-${String(date.getDate()).padStart(2,'0')} ${String(date.getHours()).padStart(2,'0')}:${String(date.getMinutes()).padStart(2,'0')}`;
}

// Global state
let currentVideoId = null;
let isFavorited = false;
let isLiked = false;
let replyTarget = null;
let hasRecordedView = false;
let currentUserEmail = "";
let commentCache = [];
let replyVisibleCount = new Map();
let commentSort = "hot";

function openUserPage(email) {
  if (!email) return;
  if (currentUserEmail && email === currentUserEmail) {
    window.location.href = "/user.html";
    return;
  }
  window.location.href = `/user.html?email=${encodeURIComponent(email)}`;
}

// Render Main Player Info
function renderPlayer(video) {
  if (!video) {
    elements.playerVideo.removeAttribute("src");
    elements.playerFallback.style.display = "flex";
    elements.playerTitle.textContent = "视频不存在或已被删除";
    return;
  }

  // Video Source
  if (video.playUrl) {
    elements.playerVideo.src = video.playUrl;
    elements.playerVideo.style.display = "block";
    elements.playerFallback.style.display = "none";
  } else {
    elements.playerVideo.removeAttribute("src");
    elements.playerFallback.style.display = "flex";
  }

  // Review status banner
  const existingBanner = document.getElementById('reviewBanner');
  if (existingBanner) existingBanner.remove();
  if (video.reviewStatus && video.reviewStatus !== 'approved') {
    const banner = document.createElement('div');
    banner.id = 'reviewBanner';
    let text = '', bg = '';
    if (video.reviewStatus === 'pending') {
      text = '此视频正在审核中，仅您本人可见';
      bg = '#f59e0b';
    } else if (video.reviewStatus === 'takedown') {
      text = '此视频已下架。原因：' + (video.takedownReason || '违反社区规范');
      bg = '#e53e3e';
    } else if (video.reviewStatus === 'rejected_violence') {
      text = '此视频因含有暴力内容未通过审核';
      bg = '#e53e3e';
    } else if (video.reviewStatus === 'rejected_nsfw') {
      text = '此视频因含有裸露内容未通过审核';
      bg = '#e53e3e';
    } else {
      text = '此视频未通过审核';
      bg = '#e53e3e';
    }
    banner.style.cssText = `background:${bg};color:#fff;padding:10px 16px;border-radius:6px;margin-bottom:12px;font-size:14px;font-weight:600;text-align:center;`;
    banner.textContent = text;
    elements.playerTitle.parentElement.insertBefore(banner, elements.playerTitle);
  }

  // Info
  elements.playerTitle.textContent = video.title;
  elements.viewCount.textContent = video.views || 0;
  elements.publishDate.textContent = formatDate(video.createdAt);
  elements.playerDesc.textContent = video.description || "-";

  // Tags
  elements.videoTags.innerHTML = "";
  if (video.tags && video.tags.length > 0) {
    video.tags.forEach(tag => {
        const span = document.createElement("span");
        span.className = "tag-item";
        span.textContent = tag;
        elements.videoTags.appendChild(span);
    });
  }

  // Uploader Info
  elements.uploaderName.textContent = video.authorNickname || video.authorEmail || "UP主";
  const mottoText = video.authorMotto || "UP主很懒，什么都没有写";
  elements.uploaderDesc.textContent = mottoText;
  
  if (video.authorAvatarUrl) {
      elements.uploaderAvatar.style.backgroundImage = `url('${video.authorAvatarUrl}')`;
      elements.uploaderAvatar.innerHTML = "";
  } else {
      // Placeholder
      const firstChar = (video.authorNickname || "U").charAt(0).toUpperCase();
      elements.uploaderAvatar.style.backgroundImage = "none";
      elements.uploaderAvatar.style.backgroundColor = "#FB7299";
      elements.uploaderAvatar.innerHTML = `<span style="position:absolute; top:50%; left:50%; transform:translate(-50%, -50%); color:#fff; font-size:20px; font-weight:bold;">${firstChar}</span>`;
  }
  
  // Favorite State
  isFavorited = video.isFavorite;
  updateFavButton();

  // Like State
  isLiked = video.isLiked;
  elements.likeCount.textContent = video.likeCount || "点赞";
  updateLikeButton();
}

function updateFavButton() {
    if (isFavorited) {
        elements.btnFav.classList.add("active-pink");
        elements.favText.textContent = "已收藏";
        // Fill icon
        const path = elements.btnFav.querySelector("path");
        if(path) path.setAttribute("d", "M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z");
    } else {
        elements.btnFav.classList.remove("active-pink");
        elements.favText.textContent = "收藏";
        // Outline icon
        const path = elements.btnFav.querySelector("path");
        if(path) path.setAttribute("d", "M16.5 3c-1.74 0-3.41.81-4.5 2.09C10.91 3.81 9.24 3 7.5 3 4.42 3 2 5.42 2 8.5c0 3.78 3.4 6.86 8.55 11.54L12 21.35l1.45-1.32C18.6 15.36 22 12.28 22 8.5 22 5.42 19.58 3 16.5 3zm-4.4 15.55l-.1.1-.1-.1C7.14 14.24 4 11.39 4 8.5 4 6.5 5.5 5 7.5 5c1.54 0 3.04.99 3.57 2.36h1.87C13.46 5.99 14.96 5 16.5 5c2 0 3.5 1.5 3.5 3.5 0 2.89-3.14 5.74-7.9 10.05z");
    }
}

function updateLikeButton() {
    if (isLiked) {
        elements.btnLike.classList.add("active");
        // Fill icon
        const path = elements.btnLike.querySelector("path");
        if(path) path.setAttribute("d", "M1 21h4V9H1v12zm22-11c0-1.1-.9-2-2-2h-6.31l.95-4.57.03-.32c0-.41-.17-.79-.44-1.06L14.17 1 7.59 7.59C7.22 7.95 7 8.45 7 9v10c0 1.1.9 2 2 2h9c.83 0 1.54-.5 1.84-1.22l3.02-7.05c.09-.23.14-.47.14-.73v-1.91l-.01-.01L23 10z");
    } else {
        elements.btnLike.classList.remove("active");
        // Outline icon
        const path = elements.btnLike.querySelector("path");
        if(path) path.setAttribute("d", "M1 21h4V9H1v12zm22-11c0-1.1-.9-2-2-2h-6.31l.95-4.57.03-.32c0-.41-.17-.79-.44-1.06L14.17 1 7.59 7.59C7.22 7.95 7 8.45 7 9v10c0 1.1.9 2 2 2h9c.83 0 1.54-.5 1.84-1.22l3.02-7.05c.09-.23.14-.47.14-.73v-1.91l-.01-.01L23 10z"); // Use same icon for now, or find outline
        // Actually the original icon I used was filled style. Let's use outline for inactive.
        if(!isLiked && path) path.setAttribute("d", "M23 10a2 2 0 0 0-2-2h-6.32l.96-4.57c.02-.1.03-.21.03-.32c0-.41-.17-.79-.44-1.06L14.17 1 7.58 7.59C7.22 7.95 7 8.45 7 9v10a2 2 0 0 0 2 2h9c.83 0 1.54-.5 1.84-1.22l3.02-7.05c.09-.23.14-.47.14-.73v-1.91l-.01-.01zm-9.9 10H9V9.3l4.98-5L12.8 9h7.11l-2.57 6H13.1v3zM1 21h4V9H1v12z");
    }
}

// Toggle Like
elements.btnLike.addEventListener("click", async () => {
    const token = getToken();
    if (!token) {
        alert("请先登录");
        window.location.href = getLoginRedirectUrl();
        return;
    }
    if (!currentVideoId) return;
    
    try {
        const res = await authFetch("/api/videos/like", {
            method: "POST",
            headers: {"Content-Type": "application/json"},
            body: JSON.stringify({ videoId: currentVideoId })
        });
        if (res.status === 401) {
            localStorage.removeItem("authToken");
            window.location.href = getLoginRedirectUrl();
            return;
        }
        if (!res.ok) throw new Error(await res.text());
        const data = await res.json();
        isLiked = data.isLiked;
        elements.likeCount.textContent = data.likeCount;
        updateLikeButton();
    } catch (err) {
        console.error("Like toggle failed", err);
        alert("操作失败: " + err.message);
    }
});

// Toggle Favorite
elements.btnFav.addEventListener("click", async () => {
    const token = getToken();
    if (!token) {
        alert("请先登录");
        window.location.href = getLoginRedirectUrl();
        return;
    }
    if (!currentVideoId) return;
    
    try {
        const res = await authFetch("/api/videos/favorite", {
            method: "POST",
            headers: {"Content-Type": "application/json"},
            body: JSON.stringify({ videoId: currentVideoId })
        });
        if (res.status === 401) {
            localStorage.removeItem("authToken");
            window.location.href = getLoginRedirectUrl();
            return;
        }
        if (!res.ok) throw new Error(await res.text());
        const data = await res.json();
        isFavorited = data.isFavorite;
        updateFavButton();
    } catch (err) {
        console.error("Favorite toggle failed", err);
        alert("操作失败: " + err.message);
    }
});

async function recordView(videoId) {
    const token = getToken();
    if (!token || !videoId || hasRecordedView) return;
    hasRecordedView = true;
    try {
        const res = await authFetch("/api/videos/view", {
            method: "POST",
            headers: {"Content-Type": "application/json"},
            body: JSON.stringify({ videoId })
        });
        if (res.status === 401) {
            localStorage.removeItem("authToken");
            return;
        }
        if (!res.ok) return;
        const data = await res.json();
        if (typeof data.viewCount === "number") {
            elements.viewCount.textContent = data.viewCount;
        }
    } catch (err) {
        console.error("View record failed", err);
    }
}

// Load Video Data
async function loadVideo() {
  const params = new URLSearchParams(window.location.search);
  const id = params.get("id");
  currentVideoId = id;
  
  if (!id) {
    renderPlayer(null);
    return;
  }

  try {
      const res = await authFetch(`/api/videos/${encodeURIComponent(id)}`);
      if (!res.ok) {
        renderPlayer(null);
        return;
      }
      const detail = await res.json();
      renderPlayer(detail);
      recordView(id);
      setupDelete(detail, id);
      setupReportBtn(detail, id);
      loadComments(id);
      loadRecommendations(detail.authorEmail);
  } catch (err) {
      console.error(err);
      renderPlayer(null);
  }
}

// Setup Delete / Manual Review Buttons
async function setupDelete(video, id) {
  const token = getToken();
  if (!token || !video || !video.authorEmail) return;

  try {
    const profileRes = await authFetch("/api/profile");
    if (!profileRes.ok) return;
    const profile = await profileRes.json();

    if (profile.email === video.authorEmail) {
      elements.deleteBtn.style.display = "inline-block";
      elements.deleteBtn.addEventListener("click", async () => {
        if (!confirm("确认要删除这个视频吗？")) return;
        const res = await authFetch(`/api/videos/${encodeURIComponent(id)}`, { method: "DELETE" });
        if (res.ok) {
          window.location.href = "/user.html";
        }
      });
      // 重新审核按钮：AI 未通过（rejected）或管理员下架（takedown）均可申请人工复审
      const canRequestReReview = video.reviewStatus && (
        video.reviewStatus === "takedown" ||
        video.reviewStatus === "rejected" ||
        String(video.reviewStatus).startsWith("rejected_")
      );
      if (canRequestReReview && elements.reReviewBtn) {
        elements.reReviewBtn.style.display = "inline-block";
        elements.reReviewBtn.addEventListener("click", async () => {
          if (!confirm("确认提交到管理员人工复审吗？")) return;
          const res = await authFetch(`/api/videos/${encodeURIComponent(id)}/manual-review`, { method: "POST" });
          let data = {};
          try {
            data = await res.json();
          } catch (e) {}
          if (!res.ok) {
            alert(data.error || "提交复审失败");
            return;
          }
          if (data.status === "already_pending") {
            alert("该视频已有待处理复审请求，请等待管理员处理");
          } else {
            alert("复审请求已提交，结果会通过消息通知");
          }
        });
      }
    }
  } catch (e) {}
}

function setupReportBtn(video, id) {
  if (!elements.reportVideoBtn) return;
  elements.reportVideoBtn.addEventListener("click", async () => {
    const token = getToken();
    if (!token) {
      window.location.href = getLoginRedirectUrl();
      return;
    }
    const reason = prompt(`请输入举报「${video.title || ""}」的理由（可选）：`);
    if (reason === null) return;
    try {
      const res = await authFetch("/api/videos/report", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ videoId: id, reason: reason })
      });
      if (res.status === 401) {
        window.location.href = getLoginRedirectUrl();
        return;
      }
      if (!res.ok) {
        const errText = await res.text();
        alert(errText || "举报失败");
        return;
      }
      await res.json();
      alert("举报成功");
    } catch (e) {
      alert("举报失败");
    }
  });
}

// 作者还制作过的视频（按作者拉取，非按分类推荐）
async function loadRecommendations(authorEmail) {
    if (!authorEmail) return;
    try {
        const res = await fetch("/api/videos?author=" + encodeURIComponent(authorEmail));
        if (!res.ok) return;
        let list = await res.json();
        list = list.filter(v => v.id !== currentVideoId);
        list = list.slice(0, 5);
        
        elements.recListContainer.innerHTML = "";
        list.forEach(v => {
            const item = document.createElement("div");
            item.className = "rec-item";
            const thumbUrl = v.thumbUrl || ""; // Add default placeholder if needed
            
            item.innerHTML = `
                <div class="rec-thumb" style="background-image: url('${thumbUrl}')"></div>
                <div class="rec-info">
                    <div class="rec-title">${v.title}</div>
                    <div class="rec-meta">UP: ${v.authorNickname || '未知'}</div>
                    <div class="rec-meta">${v.views || 0}播放</div>
                </div>
            `;
            item.addEventListener("click", () => {
                window.location.href = `/player.html?id=${encodeURIComponent(v.id)}`;
            });
            elements.recListContainer.appendChild(item);
        });
    } catch(e) {
        console.error("Failed to load recommendations", e);
    }
}

// Comments
function setCommentMsg(text, type = "") {
  elements.commentMsg.textContent = text;
  elements.commentMsg.className = `msg-box ${type}`;
  elements.commentMsg.classList.remove("hidden");
  if(type === 'success') {
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
  elements.commentList.innerHTML = "";
  elements.commentCountHeader.textContent = `(${comments ? comments.length : 0})`;
  
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
                        <span class="c-action like-btn ${reply.liked ? 'active' : ''}" data-id="${reply.id}">
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
               <span class="c-action like-btn ${item.liked ? 'active' : ''}" data-id="${item.id}">
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
                alert("请先登录");
                window.location.href = getLoginRedirectUrl();
                return;
            }
            setReplyTarget({ id: Number(btn.dataset.id), nickname: btn.dataset.nickname });
        });
    });
    card.querySelectorAll(".avatar-link, .user-link").forEach((node) => {
        node.addEventListener("click", () => {
            const email = node.dataset.email;
            if (email) openUserPage(email);
        });
    });
    card.querySelectorAll(".comment-delete").forEach((btn) => {
        btn.addEventListener("click", () => deleteComment(btn.dataset.id));
    });
    const moreBtn = card.querySelector(".reply-more");
    if (moreBtn) {
        moreBtn.addEventListener("click", () => {
            const id = Number(moreBtn.dataset.id);
            const current = replyVisibleCount.get(id) || 3;
            replyVisibleCount.set(id, current + 3);
            renderComments(commentCache);
        });
    }
    
    elements.commentList.appendChild(card);
  });
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

function setReplyTarget(target) {
    replyTarget = target;
    if (!replyTarget) {
        elements.replyHint.classList.add("hidden");
        elements.replyText.textContent = "";
        return;
    }
    elements.replyText.textContent = `回复 @${replyTarget.nickname}`;
    elements.replyHint.classList.remove("hidden");
}

async function loadComments(id) {
  try {
      const res = await authFetch(`/api/videos/${encodeURIComponent(id)}/comments`); // Use authFetch to get liked status
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
  } catch(e) {
      renderComments([]);
  }
}

async function toggleCommentLike(commentId, btn, numSpan) {
    const token = getToken();
    if (!token) {
        alert("请先登录");
        window.location.href = getLoginRedirectUrl();
        return;
    }
    try {
        const res = await authFetch(`/api/comments/${commentId}/like`, { method: "POST" });
        if (!res.ok) throw new Error();
        const data = await res.json();
        if (data.liked) btn.classList.add("active");
        else btn.classList.remove("active");
        numSpan.textContent = data.likeCount;
    } catch(e) {
        console.error(e);
    }
}

// Post Comment
elements.commentSubmit.addEventListener("click", async () => {
    if (!currentVideoId) return;
    const content = elements.commentInput.value.trim();
    if (!content) {
        setCommentMsg("请输入内容", "error");
        return;
    }
    const token = getToken();
    if (!token) {
        alert("请先登录");
        window.location.href = getLoginRedirectUrl();
        return;
    }
    
    try {
        elements.commentSubmit.disabled = true;
        const res = await authFetch(`/api/videos/${encodeURIComponent(currentVideoId)}/comments`, {
            method: "POST",
            headers: {"Content-Type": "application/json"},
            body: JSON.stringify({ content, parentId: replyTarget ? replyTarget.id : 0 })
        });
        if (!res.ok) throw new Error(await res.text());
        elements.commentInput.value = "";
        setReplyTarget(null);
        setCommentMsg("评论已提交，仅您可见，审核通过后所有人可见", "success");
        await loadComments(currentVideoId);
    } catch (err) {
        setCommentMsg(err.message || "发布失败", "error");
    } finally {
        elements.commentSubmit.disabled = false;
    }
});

elements.replyCancel.addEventListener("click", () => {
    setReplyTarget(null);
});

async function deleteComment(commentId) {
    const token = getToken();
    if (!token) {
        alert("请先登录");
        window.location.href = getLoginRedirectUrl();
        return;
    }
    if (!confirm("确定删除该评论吗？")) return;
    try {
        const res = await authFetch(`/api/comments/${commentId}`, { method: "DELETE" });
        if (res.status === 401) {
            localStorage.removeItem("authToken");
            window.location.href = getLoginRedirectUrl();
            return;
        }
        if (!res.ok) throw new Error(await res.text());
        await loadComments(currentVideoId);
    } catch (err) {
        alert("删除失败");
    }
}

// Load My Avatar for Comment Box
async function loadMyAvatar() {
    const token = getToken();
    if (!token) return;
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
    } catch(e){}
}

// Message badge
function loadMsgBadge() {
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
}

// Scroll to comment from hash
function scrollToCommentFromHash() {
  const hash = window.location.hash;
  if (hash && hash.startsWith("#comment-")) {
    const commentId = hash.replace("#comment-", "");
    // Wait for comments to load, then scroll
    const tryScroll = (retries) => {
      const el = document.querySelector(`[data-comment-id="${commentId}"]`);
      if (el) {
        // Expand comment-list scroll to show all
        const commentListEl = document.getElementById("commentList");
        if (commentListEl) {
          commentListEl.style.maxHeight = "none";
        }
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

// Initialize
initHeader();
loadMsgBadge();
loadVideo();
loadMyAvatar();
