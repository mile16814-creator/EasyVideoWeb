(function() {
  var evwLangResolve;
  window.evwLangPromise = new Promise(function(r) { evwLangResolve = r; });
  var locale = localStorage.getItem('evw_locale') || 'zh-CN';
  function setT(lang) {
    window.evwLang = lang || {};
    window.evwT = function(key) {
      var keys = key.split('.');
      var v = window.evwLang;
      for (var i = 0; i < keys.length; i++) v = v != null ? v[keys[i]] : undefined;
      return v != null ? String(v) : key;
    };
  }
  fetch('/language/' + locale + '.json')
    .then(function(r) {
      if (r.ok) return r.json();
      return fetch('/language/zh-CN.json').then(function(r2) { return r2.ok ? r2.json() : {}; });
    })
    .then(function(lang) {
      setT(lang);
      if (evwLangResolve) evwLangResolve();
    })
    .catch(function() {
      setT({});
      if (evwLangResolve) evwLangResolve();
    });
})();
