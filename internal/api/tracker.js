/**
 * Etiquetta Analytics Tracker
 * Privacy-first, cookie-free analytics.
 */
(function() {
  "use strict";

  if (window.__ETIQUETTA_LOADED__) return;
  window.__ETIQUETTA_LOADED__ = true;

  // Configuration
  const CONFIG = window.__ETIQUETTA_CONFIG__ || {};

  function getScript() {
    const scripts = document.querySelectorAll('script[src*="s.js"]');
    for (let i = 0; i < scripts.length; i++) {
      const src = scripts[i].src;
      if (src && src.includes('s.js')) {
        const url = new URL(src);
        return { baseUrl: url.origin, siteId: scripts[i].getAttribute('data-site') };
      }
    }
    return { baseUrl: location.origin, siteId: null };
  }

  const SCRIPT = getScript();
  const BASE_URL = CONFIG.baseUrl || SCRIPT.baseUrl;
  const INGEST_URL = BASE_URL + (CONFIG.endpoint || "/i");
  const SITE_ID = CONFIG.siteId || SCRIPT.siteId;
  const DOMAIN = location.hostname;
  const DEBUG = CONFIG.debug || false;
  const TRACK_PERFORMANCE = CONFIG.trackPerformance !== false;
  const TRACK_ERRORS = CONFIG.trackErrors !== false;

  // Rate limiting
  let eventCount = 0;
  let rateWindowStart = Date.now();
  const RATE_LIMIT = 100;
  const RATE_WINDOW = 60000;

  // Event queue
  let queue = [];
  let flushTimer = null;
  const FLUSH_INTERVAL = 1000;
  const MAX_BATCH = 10;

  // Behavioral state
  const BEHAVIOR = {
    scrolled: false,
    moused: false,
    clicked: false,
    touched: false,
    clickX: null,
    clickY: null
  };

  // MurmurHash3 (32-bit)
  function hash(key, seed) {
    seed = seed || 0;
    const c1 = 0xcc9e2d51, c2 = 0x1b873593;
    let h1 = seed, len = key.length, i = 0;

    while (i + 4 <= len) {
      let k1 = (key.charCodeAt(i) & 0xff) |
        ((key.charCodeAt(i + 1) & 0xff) << 8) |
        ((key.charCodeAt(i + 2) & 0xff) << 16) |
        ((key.charCodeAt(i + 3) & 0xff) << 24);
      k1 = Math.imul(k1, c1);
      k1 = (k1 << 15) | (k1 >>> 17);
      k1 = Math.imul(k1, c2);
      h1 ^= k1;
      h1 = (h1 << 13) | (h1 >>> 19);
      h1 = Math.imul(h1, 5) + 0xe6546b64;
      i += 4;
    }

    let k1 = 0;
    switch (len & 3) {
      case 3: k1 ^= (key.charCodeAt(i + 2) & 0xff) << 16;
      case 2: k1 ^= (key.charCodeAt(i + 1) & 0xff) << 8;
      case 1:
        k1 ^= key.charCodeAt(i) & 0xff;
        k1 = Math.imul(k1, c1);
        k1 = (k1 << 15) | (k1 >>> 17);
        k1 = Math.imul(k1, c2);
        h1 ^= k1;
    }

    h1 ^= len;
    h1 ^= h1 >>> 16;
    h1 = Math.imul(h1, 0x85ebca6b);
    h1 ^= h1 >>> 13;
    h1 = Math.imul(h1, 0xc2b2ae35);
    h1 ^= h1 >>> 16;
    return (h1 >>> 0).toString(16).padStart(8, "0");
  }

  function log(...args) {
    if (DEBUG) console.log("[Etiquetta]", ...args);
  }

  function uuid() {
    if (crypto && crypto.randomUUID) return crypto.randomUUID().replace(/-/g, "");
    return "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx".replace(/x/g, () => ((Math.random() * 16) | 0).toString(16));
  }

  // Simple visitor hash (fast, no expensive fingerprinting)
  function getVisitorHash() {
    const signals = [
      screen.width, screen.height, screen.colorDepth,
      new Date().getTimezoneOffset(),
      navigator.language,
      navigator.hardwareConcurrency || 0,
      navigator.platform || ""
    ].join("|");
    return hash(signals, 0) + hash(signals, 1);
  }

  // Visitor identification based on tracking mode
  const TRACKING_MODE = CONFIG.trackingMode || "cookieless";
  const VID_KEY = "_etq_vid";

  function getCookie(name) {
    var match = document.cookie.match(new RegExp("(?:^|; )" + name + "=([^;]*)"));
    return match ? decodeURIComponent(match[1]) : null;
  }

  function setCookie(name, value, days) {
    var d = new Date();
    d.setTime(d.getTime() + days * 864e5);
    document.cookie = name + "=" + encodeURIComponent(value) + ";expires=" + d.toUTCString() + ";path=/;SameSite=Lax";
  }

  function deleteCookie(name) {
    document.cookie = name + "=;expires=Thu, 01 Jan 1970 00:00:00 GMT;path=/;SameSite=Lax";
  }

  function getOrCreateVisitorId() {
    if (TRACKING_MODE === "cookie") {
      try { localStorage.removeItem(VID_KEY); } catch(e) {}
      var existing = getCookie(VID_KEY);
      if (existing) return existing;
      var vid = uuid();
      setCookie(VID_KEY, vid, 365);
      return vid;
    }
    if (TRACKING_MODE === "localStorage") {
      deleteCookie(VID_KEY);
      try {
        var existing = localStorage.getItem(VID_KEY);
        if (existing) return existing;
        var vid = uuid();
        localStorage.setItem(VID_KEY, vid);
        return vid;
      } catch(e) { /* blocked — fall through to cookieless */ }
    }
    // Cookieless: clean up any previous persistence
    deleteCookie(VID_KEY);
    try { localStorage.removeItem(VID_KEY); } catch(e) {}
    return getVisitorHash();
  }

  var VISITOR_HASH = getOrCreateVisitorId();

  // Bot detection signals (weight checks)
  function getBotSignals() {
    return {
      webdriver: navigator.webdriver ? 1 : 0,
      phantom: (window._phantom || window.callPhantom) ? 1 : 0,
      selenium: (window._selenium || window.__selenium_unwrapped || document.__selenium_unwrapped) ? 1 : 0,
      headless: /HeadlessChrome/.test(navigator.userAgent) ? 1 : 0,
      screen_valid: (screen.width > 0 && screen.height > 0) ? 1 : 0,
      screen_width: screen.width || 0,
      screen_height: screen.height || 0,
      plugins: navigator.plugins ? navigator.plugins.length : 0,
      languages: navigator.languages ? navigator.languages.length : 0,
      cdp_detected: (function() {
        try {
          for (var key in document) {
            if (/^cdc_/.test(key) || /^__webdriver/.test(key)) return 1;
          }
        } catch(e) {}
        return 0;
      })(),
      doc_hidden_at_load: document.hidden ? 1 : 0
    };
  }

  // Event sending
  function checkRateLimit() {
    const now = Date.now();
    if (now - rateWindowStart > RATE_WINDOW) {
      eventCount = 0;
      rateWindowStart = now;
    }
    if (eventCount >= RATE_LIMIT) return false;
    eventCount++;
    return true;
  }

  function queueEvent(type, data) {
    if (!checkRateLimit()) return;
    queue.push({ type, data, ts: Date.now() });
    if (queue.length >= MAX_BATCH) {
      flush();
    } else if (!flushTimer) {
      flushTimer = setTimeout(flush, FLUSH_INTERVAL);
    }
  }

  // Payload encoding helpers
  var isUnloading = false;

  function xorEncode(str) {
    var key = [0x4e, 0x71, 0x63, 0x58];
    var bytes = new Uint8Array(str.length);
    for (var i = 0; i < str.length; i++) {
      bytes[i] = str.charCodeAt(i) ^ key[i & 3];
    }
    var binary = "";
    for (var j = 0; j < bytes.length; j++) {
      binary += String.fromCharCode(bytes[j]);
    }
    return btoa(binary);
  }

  function sendBlob(blob, type) {
    if (navigator.sendBeacon) {
      navigator.sendBeacon(INGEST_URL, new Blob([blob], { type: type }));
    } else {
      fetch(INGEST_URL, { method: "POST", body: blob, keepalive: true, headers: { "Content-Type": type } }).catch(function() {});
    }
  }

  function sendXor(ndjson) {
    var encoded = xorEncode(ndjson);
    sendBlob(encoded, "text/x-etq");
  }

  function flush() {
    if (flushTimer) {
      clearTimeout(flushTimer);
      flushTimer = null;
    }
    if (!queue.length) return;

    var batch = queue.splice(0, MAX_BATCH);
    var ndjson = batch.map(function(e) { return JSON.stringify({ type: e.type, ...e.data }); }).join("\n");

    // During page unload, use sync XOR path (CompressionStream is async and may not resolve)
    if (!isUnloading && typeof CompressionStream !== "undefined") {
      new Response(new Blob([ndjson]).stream().pipeThrough(new CompressionStream("deflate"))).blob().then(function(compressed) {
        sendBlob(compressed, "application/octet-stream");
      }).catch(function() {
        sendXor(ndjson);
      });
    } else {
      sendXor(ndjson);
    }
    log("Flushed", batch.length, "events");
  }

  function send(table, data, withSignals = false) {
    const event = {
      event_id: uuid(),
      timestamp: Date.now(),
      site_id: SITE_ID,
      domain: DOMAIN,
      visitor_hash: VISITOR_HASH,
      ...data
    };

    // Always send bot signals so performance/error events can be correlated
    event.bot_signals = getBotSignals();

    if (withSignals || table === "events") {
      event.has_scroll = BEHAVIOR.scrolled ? 1 : 0;
      event.has_mouse_move = BEHAVIOR.moused ? 1 : 0;
      event.has_click = BEHAVIOR.clicked ? 1 : 0;
      event.has_touch = BEHAVIOR.touched ? 1 : 0;
      if (BEHAVIOR.clickX !== null) {
        event.click_x = BEHAVIOR.clickX;
        event.click_y = BEHAVIOR.clickY;
      }
    }

    queueEvent(table, event);
    log("Queued:", table, event);
  }

  // Behavioral tracking
  function setupBehavior() {
    window.addEventListener("scroll", () => { BEHAVIOR.scrolled = true; }, { passive: true, once: true });
    window.addEventListener("mousemove", () => { BEHAVIOR.moused = true; }, { passive: true, once: true });
    window.addEventListener("touchstart", () => { BEHAVIOR.touched = true; }, { passive: true, once: true });
    document.addEventListener("click", (e) => {
      BEHAVIOR.clicked = true;
      BEHAVIOR.clickX = e.clientX;
      BEHAVIOR.clickY = e.clientY;
    }, { capture: true, passive: true });
  }

  // Pageview tracking
  let lastPage = null;

  function trackPageview(opts = {}) {
    const url = opts.url || location.href;
    if (lastPage === url && !opts.force) return;
    lastPage = url;

    const u = new URL(url);
    send("events", {
      event_type: "pageview",
      event_name: "pv",
      url: url,
      path: u.pathname,
      referrer_url: document.referrer || null,
      page_title: document.title || null,
      utm_source: u.searchParams.get("utm_source"),
      utm_medium: u.searchParams.get("utm_medium"),
      utm_campaign: u.searchParams.get("utm_campaign")
    });
  }

  // SPA navigation
  function setupSPA() {
    const push = history.pushState;
    const replace = history.replaceState;
    let navTimer = null;

    function onNav() {
      if (navTimer) clearTimeout(navTimer);
      navTimer = setTimeout(() => trackPageview(), 50);
    }

    history.pushState = function() { push.apply(this, arguments); onNav(); };
    history.replaceState = function() { replace.apply(this, arguments); onNav(); };
    window.addEventListener("popstate", onNav);
  }

  // Scroll tracking (milestones)
  function setupScroll() {
    const milestones = [25, 50, 75, 100];
    const reached = new Set();
    let ticking = false;

    window.addEventListener("scroll", () => {
      if (ticking) return;
      ticking = true;
      requestAnimationFrame(() => {
        const h = document.documentElement.scrollHeight - window.innerHeight;
        if (h > 0) {
          const pct = Math.round((window.scrollY / h) * 100);
          for (const m of milestones) {
            if (pct >= m && !reached.has(m)) {
              reached.add(m);
              send("events", {
                event_type: "scroll",
                event_name: "scroll_" + m,
                url: location.href,
                path: location.pathname
              });
            }
          }
        }
        ticking = false;
      });
    }, { passive: true });

    window.addEventListener("popstate", () => reached.clear());
  }

  // Outbound link tracking
  function setupOutbound() {
    document.addEventListener("click", (e) => {
      const link = e.target.closest("a");
      if (link && link.href) {
        try {
          const u = new URL(link.href);
          if (u.host !== location.host) {
            send("events", {
              event_type: "click",
              event_name: "outbound",
              url: location.href,
              path: location.pathname,
              props: JSON.stringify({ target: link.href })
            });
          }
        } catch (err) {}
      }
    }, { capture: true });
  }

  // Error tracking (capped to prevent memory leaks on long-lived SPAs)
  const seenErrors = new Set();
  const SEEN_ERRORS_MAX = 200;

  function setupErrors() {
    if (!TRACK_ERRORS) return;

    window.addEventListener("error", (e) => {
      if (e.filename) {
        const fp = hash(e.message + "|" + e.filename + "|" + e.lineno + "|" + e.colno);
        if (seenErrors.has(fp)) return;
        if (seenErrors.size >= SEEN_ERRORS_MAX) seenErrors.clear();
        seenErrors.add(fp);
        send("error", {
          error_type: "javascript",
          error_hash: fp,
          url: location.href,
          path: location.pathname,
          error_message: (e.message || "").substring(0, 500),
          script_url: e.filename || "",
          line_number: e.lineno || 0,
          column_number: e.colno || 0,
          error_stack: e.error ? (e.error.stack || "").substring(0, 2000) : ""
        });
      }
    }, true);

    window.addEventListener("unhandledrejection", (e) => {
      const reason = e.reason || {};
      const msg = reason.message || String(reason);
      const fp = hash(msg);
      if (seenErrors.has(fp)) return;
      seenErrors.add(fp);
      send("error", {
        error_type: "unhandled_rejection",
        error_hash: fp,
        url: location.href,
        path: location.pathname,
        error_message: msg.substring(0, 500),
        error_stack: reason.stack ? reason.stack.substring(0, 2000) : ""
      });
    });
  }

  // Performance tracking (Core Web Vitals)
  const PERF = { lcp: null, fcp: null, cls: null, inp: null, ttfb: null, pageLoad: null, sent: false };

  function setupPerformance() {
    if (!TRACK_PERFORMANCE) return;

    if (document.readyState === "complete") {
      collectTiming();
    } else {
      window.addEventListener("load", () => setTimeout(collectTiming, 100));
    }

    observeVitals();

    document.addEventListener("visibilitychange", () => {
      if (document.visibilityState === "hidden") sendPerf();
    });
    window.addEventListener("pagehide", sendPerf);
  }

  function collectTiming() {
    const nav = performance.getEntriesByType("navigation")[0];
    if (nav) {
      PERF.pageLoad = Math.round(nav.loadEventEnd);
      PERF.ttfb = Math.round(nav.responseStart);
    }
  }

  function observeVitals() {
    if (!window.PerformanceObserver) return;

    try {
      new PerformanceObserver((list) => {
        const e = list.getEntries();
        PERF.lcp = Math.round(e[e.length - 1].startTime);
      }).observe({ type: "largest-contentful-paint", buffered: true });
    } catch (e) {}

    try {
      new PerformanceObserver((list) => {
        for (const e of list.getEntries()) {
          if (e.name === "first-contentful-paint") PERF.fcp = Math.round(e.startTime);
        }
      }).observe({ type: "paint", buffered: true });
    } catch (e) {}

    try {
      let cls = 0;
      new PerformanceObserver((list) => {
        for (const e of list.getEntries()) {
          if (!e.hadRecentInput) cls += e.value;
        }
        PERF.cls = Math.round(cls * 1000) / 1000;
      }).observe({ type: "layout-shift", buffered: true });
    } catch (e) {}

    try {
      let maxInp = 0;
      new PerformanceObserver((list) => {
        for (const e of list.getEntries()) {
          if (e.duration > maxInp) maxInp = e.duration;
        }
        PERF.inp = Math.round(maxInp);
      }).observe({ type: "event", buffered: true, durationThreshold: 16 });
    } catch (e) {}
  }

  function sendPerf() {
    if (PERF.sent) return;
    PERF.sent = true;
    send("performance", {
      url: location.href,
      path: location.pathname,
      lcp: PERF.lcp,
      fcp: PERF.fcp,
      cls: PERF.cls,
      inp: PERF.inp,
      ttfb: PERF.ttfb,
      page_load_time: PERF.pageLoad,
      connection_type: navigator.connection?.effectiveType || null
    });
    flush();
  }

  // Engagement tracking
  const ENG = {
    loadTime: Date.now(),
    lastVisible: Date.now(),
    visibleTime: 0,
    visible: !document.hidden,
    maxScroll: 0,
    exitSent: false
  };

  function setupEngagement() {
    window.addEventListener("scroll", () => {
      const h = document.documentElement.scrollHeight - window.innerHeight;
      if (h > 0) {
        const pct = Math.round((window.scrollY / h) * 100);
        if (pct > ENG.maxScroll) ENG.maxScroll = pct;
      }
    }, { passive: true });

    document.addEventListener("visibilitychange", () => {
      const now = Date.now();
      if (document.visibilityState === "visible") {
        ENG.lastVisible = now;
        ENG.visible = true;
      } else {
        if (ENG.visible) ENG.visibleTime += now - ENG.lastVisible;
        ENG.visible = false;
        flush();
      }
    });

    window.addEventListener("pagehide", sendExit);
    window.addEventListener("beforeunload", sendExit);
  }

  function sendExit() {
    if (ENG.exitSent) return;
    ENG.exitSent = true;

    const now = Date.now();
    if (ENG.visible) ENG.visibleTime += now - ENG.lastVisible;
    const total = now - ENG.loadTime;

    send("events", {
      event_type: "engagement",
      event_name: "page_exit",
      url: location.href,
      path: location.pathname,
      page_duration: total,
      props: JSON.stringify({
        total_time_ms: total,
        visible_time_ms: ENG.visibleTime,
        scroll_depth: ENG.maxScroll
      })
    }, true);
    flush();
  }

  // Custom event API
  function track(name, props) {
    if (!name || typeof name !== "string") return;
    send("events", {
      event_type: "custom",
      event_name: name,
      url: location.href,
      path: location.pathname,
      props: JSON.stringify(props || {})
    });
  }

  // Public API
  window.etiquetta = {
    track: track,
    pageview: trackPageview,
    flush: flush,
    getVisitorHash: () => VISITOR_HASH
  };

  // Init
  function startTracking() {
    log("Etiquetta v2.0 initializing");
    setupBehavior();
    setupSPA();
    setupScroll();
    setupOutbound();
    setupErrors();
    setupPerformance();
    setupEngagement();

    if (document.readyState === "loading") {
      document.addEventListener("DOMContentLoaded", trackPageview);
    } else {
      trackPageview();
    }

    window.addEventListener("beforeunload", function() { isUnloading = true; flush(); });
    window.addEventListener("pagehide", function() { isUnloading = true; flush(); });

    // bfcache: when page is restored from back-forward cache, re-track pageview
    window.addEventListener("pageshow", function(e) {
      if (e.persisted) {
        isUnloading = false;
        trackPageview({ force: true });
        log("Page restored from bfcache");
      }
    });

    log("Etiquetta initialized");
  }

  function init() {
    // Respect DNT / GPC if configured
    if (CONFIG.respectDNT !== false) {
      if (navigator.doNotTrack === "1" || navigator.globalPrivacyControl === true) {
        log("DNT/GPC signal detected, not tracking");
        window.etiquetta = { track: function(){}, pageview: function(){}, flush: function(){}, getVisitorHash: function(){ return ""; } };
        return;
      }
    }

    var consent = window.__ETIQUETTA_CONSENT__;

    // If consent system is loaded and analytics is explicitly denied, wait
    if (consent && consent.analytics === false) {
      log("Analytics consent not granted, waiting...");
      window.addEventListener("etiquetta:consent", function handler(e) {
        var c = window.__ETIQUETTA_CONSENT__;
        if (c && c.analytics !== false) {
          window.removeEventListener("etiquetta:consent", handler);
          startTracking();
        }
      });
      return;
    }

    // Defer tracking if page is prerendering (Speculation Rules API)
    if (document.prerendering) {
      log("Page is prerendering, deferring tracking");
      document.addEventListener("prerenderingchange", function() { startTracking(); }, { once: true });
      return;
    }

    // Either consent granted or no consent system loaded (backwards compatible)
    startTracking();
  }

  init();

  // Auto-load consent banner, tag manager, and session recorder
  // Only tracker script tag is required; consent (c.js), tag manager (tm/{siteId}.js),
  // and recorder (r.js) are injected automatically. All handle 404s gracefully.
  if (SITE_ID) {
    function loadRecorder() {
      var rs = document.createElement('script');
      rs.src = BASE_URL + '/r.js';
      rs.async = true;
      document.head.appendChild(rs);
    }

    function loadTagManager() {
      var tms = document.createElement('script');
      var tmUrl = BASE_URL + '/tm/' + SITE_ID + '.js';
      var dbgToken = new URLSearchParams(location.search).get('etq_debug');
      if (dbgToken) tmUrl += '?debug=' + encodeURIComponent(dbgToken);
      tms.src = tmUrl;
      tms.async = true;
      tms.onload = loadRecorder;
      tms.onerror = loadRecorder;
      document.head.appendChild(tms);
    }

    if (!window.__ETIQUETTA_CONSENT_LOADED__) {
      var cs = document.createElement('script');
      cs.src = BASE_URL + '/c.js';
      cs.setAttribute('data-site', SITE_ID);
      cs.async = true;
      cs.onload = loadTagManager;
      cs.onerror = loadTagManager; // Load TM even if consent fails (404 = not configured)
      document.head.appendChild(cs);
    } else {
      loadTagManager();
    }
  }
})();
