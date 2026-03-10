/**
 * Etiquetta Consent Manager
 * Privacy-first consent banner for GDPR compliance.
 */
(function() {
  "use strict";

  if (window.__ETIQUETTA_CONSENT_LOADED__) return;
  window.__ETIQUETTA_CONSENT_LOADED__ = true;

  // Get script config
  function getScript() {
    var scripts = document.querySelectorAll('script[src*="c.js"]');
    for (var i = 0; i < scripts.length; i++) {
      var src = scripts[i].src;
      if (src && src.includes('c.js')) {
        var url = new URL(src);
        return { baseUrl: url.origin, siteId: scripts[i].getAttribute('data-site') };
      }
    }
    return { baseUrl: location.origin, siteId: null };
  }

  var SCRIPT = getScript();
  if (!SCRIPT.siteId) return;

  var BASE_URL = SCRIPT.baseUrl;
  var SITE_ID = SCRIPT.siteId;
  var COOKIE_PREFIX = 'etiquetta_consent_';

  // Cookie helpers
  function getCookie(name) {
    var match = document.cookie.match(new RegExp('(^| )' + name + '=([^;]+)'));
    return match ? match[2] : null;
  }

  function setCookie(name, value, days) {
    var d = new Date();
    d.setTime(d.getTime() + days * 86400000);
    document.cookie = name + '=' + value + ';expires=' + d.toUTCString() + ';path=/;SameSite=Lax';
  }

  function parseConsent(raw) {
    try { return JSON.parse(atob(raw)); } catch (e) { return null; }
  }

  function encodeConsent(obj) {
    return btoa(JSON.stringify(obj));
  }

  // Check existing cookie
  var cookieName = COOKIE_PREFIX + SITE_ID;
  var existing = getCookie(cookieName);
  var parsed = existing ? parseConsent(existing) : null;

  function setConsentState(categories) {
    window.__ETIQUETTA_CONSENT__ = categories;
    try {
      window.dispatchEvent(new CustomEvent('etiquetta:consent', { detail: categories }));
    } catch(e) {
      // IE fallback
      var evt = document.createEvent('CustomEvent');
      evt.initCustomEvent('etiquetta:consent', true, true, categories);
      window.dispatchEvent(evt);
    }
  }

  // If valid consent cookie exists, apply it and check version
  if (parsed && parsed.c) {
    setConsentState(parsed.c);
    // We'll check version after fetching config
  }

  // Fetch config
  function fetchConfig(callback) {
    var xhr = new XMLHttpRequest();
    xhr.open('GET', BASE_URL + '/consent/' + SITE_ID + '/config', true);
    xhr.onreadystatechange = function() {
      if (xhr.readyState === 4) {
        if (xhr.status === 200) {
          try {
            callback(JSON.parse(xhr.responseText));
          } catch(e) {
            callback(null);
          }
        } else {
          callback(null); // 404 = consent not configured, exit silently
        }
      }
    };
    xhr.send();
  }

  function recordConsent(action, categories, configVersion) {
    var body = JSON.stringify({
      visitor_hash: getVisitorHash(),
      categories: categories,
      config_version: configVersion,
      action: action
    });
    if (navigator.sendBeacon) {
      navigator.sendBeacon(BASE_URL + '/consent/' + SITE_ID + '/record', new Blob([body], {type: 'application/json'}));
    } else {
      var xhr = new XMLHttpRequest();
      xhr.open('POST', BASE_URL + '/consent/' + SITE_ID + '/record', true);
      xhr.setRequestHeader('Content-Type', 'application/json');
      xhr.send(body);
    }
  }

  // Simple visitor hash (matches tracker.js pattern)
  function getVisitorHash() {
    var s = [screen.width, screen.height, screen.colorDepth, new Date().getTimezoneOffset(), navigator.language, navigator.hardwareConcurrency || 0, navigator.platform || ''].join('|');
    // Simple hash
    var h = 0;
    for (var i = 0; i < s.length; i++) { h = ((h << 5) - h) + s.charCodeAt(i); h |= 0; }
    return Math.abs(h).toString(16);
  }

  // Detect language
  function detectLang() {
    var lang = (navigator.language || navigator.userLanguage || 'en').split('-')[0].toLowerCase();
    return lang;
  }

  // Get translated text
  function t(config, key, lang) {
    if (config.translations && config.translations[lang] && config.translations[lang][key]) {
      return config.translations[lang][key];
    }
    // Defaults
    var defaults = {
      title: 'Cookie Consent',
      description: 'We use cookies to enhance your experience. By continuing to visit this site you agree to our use of cookies.',
      accept_all: 'Accept All',
      reject_all: 'Reject All',
      customize: 'Customize',
      save_preferences: 'Save Preferences',
      close: 'Close'
    };
    return defaults[key] || key;
  }

  // Render banner
  function renderBanner(config) {
    var app = config.appearance || {};
    var style = app.style || 'bar'; // bar, popup, modal
    var position = app.position || 'bottom'; // top, bottom, bottom-left, bottom-right, center
    var bgColor = app.bg_color || '#ffffff';
    var textColor = app.text_color || '#1a1a1a';
    var btnBgColor = app.btn_bg_color || '#000000';
    var btnTextColor = app.btn_text_color || '#ffffff';
    var showRejectAll = app.show_reject_all !== false;
    var showCustomize = (config.categories || []).length > 1;
    var lang = config.auto_language ? detectLang() : 'en';
    var categories = config.categories || [];
    var configVersion = config.version || 1;
    var cookieExpiry = config.cookie_expiry_days || 365;

    // Container
    var overlay = document.createElement('div');
    overlay.id = 'etiquetta-consent';

    // Base styles for overlay
    var overlayStyles = 'position:fixed;z-index:999999;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;font-size:14px;line-height:1.5;';

    if (style === 'modal') {
      overlayStyles += 'top:0;left:0;right:0;bottom:0;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,0.5);';
    } else if (style === 'popup') {
      if (position === 'bottom-left') overlayStyles += 'bottom:20px;left:20px;';
      else if (position === 'bottom-right' || position === 'bottom') overlayStyles += 'bottom:20px;right:20px;';
      else overlayStyles += 'top:20px;right:20px;';
    } else { // bar
      if (position === 'top') overlayStyles += 'top:0;left:0;right:0;';
      else overlayStyles += 'bottom:0;left:0;right:0;';
    }
    overlay.setAttribute('style', overlayStyles);

    // Banner card
    var card = document.createElement('div');
    var cardStyles = 'background:' + bgColor + ';color:' + textColor + ';padding:20px;';
    if (style === 'bar') {
      cardStyles += 'box-shadow:0 -2px 10px rgba(0,0,0,0.1);display:flex;flex-wrap:wrap;align-items:center;gap:16px;';
    } else if (style === 'popup') {
      cardStyles += 'border-radius:12px;box-shadow:0 4px 20px rgba(0,0,0,0.15);max-width:400px;width:100%;';
    } else { // modal
      cardStyles += 'border-radius:12px;box-shadow:0 4px 20px rgba(0,0,0,0.15);max-width:500px;width:90%;max-height:80vh;overflow-y:auto;';
    }
    card.setAttribute('style', cardStyles);

    // Main view
    var mainHTML = '<div style="flex:1;min-width:200px;">';
    mainHTML += '<strong style="display:block;margin-bottom:4px;">' + t(config, 'title', lang) + '</strong>';
    mainHTML += '<p style="margin:0;opacity:0.8;font-size:13px;">' + t(config, 'description', lang) + '</p>';
    mainHTML += '</div>';
    mainHTML += '<div style="display:flex;gap:8px;flex-wrap:wrap;' + (style !== 'bar' ? 'margin-top:16px;' : '') + '">';
    if (showRejectAll) {
      mainHTML += '<button data-action="reject" style="padding:8px 20px;border-radius:6px;border:1px solid ' + textColor + ';background:transparent;color:' + textColor + ';cursor:pointer;font-size:14px;font-weight:500;">' + t(config, 'reject_all', lang) + '</button>';
    }
    if (showCustomize) {
      mainHTML += '<button data-action="customize" style="padding:8px 20px;border-radius:6px;border:1px solid ' + textColor + ';background:transparent;color:' + textColor + ';cursor:pointer;font-size:14px;font-weight:500;">' + t(config, 'customize', lang) + '</button>';
    }
    mainHTML += '<button data-action="accept" style="padding:8px 20px;border-radius:6px;border:none;background:' + btnBgColor + ';color:' + btnTextColor + ';cursor:pointer;font-size:14px;font-weight:500;">' + t(config, 'accept_all', lang) + '</button>';
    mainHTML += '</div>';

    // Customize view
    var customizeHTML = '<div style="display:none;" data-view="customize">';
    customizeHTML += '<strong style="display:block;margin-bottom:12px;">' + t(config, 'title', lang) + '</strong>';
    for (var i = 0; i < categories.length; i++) {
      var cat = categories[i];
      var isRequired = cat.required === true;
      customizeHTML += '<div style="display:flex;align-items:center;justify-content:space-between;padding:8px 0;border-bottom:1px solid rgba(0,0,0,0.1);">';
      customizeHTML += '<div><strong style="font-size:13px;">' + (cat.label || cat.id) + '</strong>';
      if (cat.description) customizeHTML += '<p style="margin:2px 0 0;font-size:12px;opacity:0.7;">' + cat.description + '</p>';
      customizeHTML += '</div>';
      customizeHTML += '<label style="position:relative;display:inline-block;width:44px;height:24px;flex-shrink:0;margin-left:12px;">';
      customizeHTML += '<input type="checkbox" data-cat="' + cat.id + '"' + (isRequired || cat.default_enabled !== false ? ' checked' : '') + (isRequired ? ' disabled' : '') + ' style="opacity:0;width:0;height:0;">';
      customizeHTML += '<span style="position:absolute;cursor:' + (isRequired ? 'not-allowed' : 'pointer') + ';top:0;left:0;right:0;bottom:0;background:' + (isRequired || cat.default_enabled !== false ? btnBgColor : '#ccc') + ';border-radius:24px;transition:0.3s;"></span>';
      customizeHTML += '<span style="position:absolute;content:\'\';height:18px;width:18px;left:' + (isRequired || cat.default_enabled !== false ? '22px' : '3px') + ';bottom:3px;background:white;border-radius:50%;transition:0.3s;"></span>';
      customizeHTML += '</label></div>';
    }
    customizeHTML += '<div style="display:flex;gap:8px;margin-top:16px;">';
    customizeHTML += '<button data-action="save" style="flex:1;padding:8px 20px;border-radius:6px;border:none;background:' + btnBgColor + ';color:' + btnTextColor + ';cursor:pointer;font-size:14px;font-weight:500;">' + t(config, 'save_preferences', lang) + '</button>';
    customizeHTML += '</div></div>';

    card.innerHTML = '<div data-view="main">' + mainHTML + '</div>' + customizeHTML;
    overlay.appendChild(card);

    // Toggle logic for switches
    function setupToggles() {
      var checkboxes = card.querySelectorAll('input[type="checkbox"][data-cat]');
      for (var i = 0; i < checkboxes.length; i++) {
        (function(cb) {
          cb.addEventListener('change', function() {
            var slider = cb.nextElementSibling;
            var dot = slider.nextElementSibling;
            if (cb.checked) {
              slider.style.background = btnBgColor;
              dot.style.left = '22px';
            } else {
              slider.style.background = '#ccc';
              dot.style.left = '3px';
            }
          });
        })(checkboxes[i]);
      }
    }

    // Actions
    function handleAction(action) {
      var cats = {};
      if (action === 'accept') {
        for (var i = 0; i < categories.length; i++) cats[categories[i].id] = true;
        saveAndClose('accept_all', cats);
      } else if (action === 'reject') {
        for (var i = 0; i < categories.length; i++) {
          cats[categories[i].id] = categories[i].required === true;
        }
        saveAndClose('reject_all', cats);
      } else if (action === 'customize') {
        card.querySelector('[data-view="main"]').style.display = 'none';
        card.querySelector('[data-view="customize"]').style.display = 'block';
        setupToggles();
      } else if (action === 'save') {
        var checkboxes = card.querySelectorAll('input[type="checkbox"][data-cat]');
        for (var i = 0; i < checkboxes.length; i++) {
          cats[checkboxes[i].getAttribute('data-cat')] = checkboxes[i].checked;
        }
        saveAndClose('custom', cats);
      }
    }

    function saveAndClose(action, cats) {
      // Set cookie
      var cookieVal = encodeConsent({ v: configVersion, c: cats, t: Date.now(), u: Date.now() });
      setCookie(cookieName, cookieVal, cookieExpiry);

      // Record consent server-side
      recordConsent(action, cats, configVersion);

      // Set global state
      setConsentState(cats);

      // Remove banner
      if (overlay.parentNode) overlay.parentNode.removeChild(overlay);
    }

    // Event delegation
    card.addEventListener('click', function(e) {
      var btn = e.target.closest ? e.target.closest('[data-action]') : null;
      if (!btn) {
        // Fallback for older browsers
        var el = e.target;
        while (el && el !== card) {
          if (el.getAttribute && el.getAttribute('data-action')) { btn = el; break; }
          el = el.parentNode;
        }
      }
      if (btn) handleAction(btn.getAttribute('data-action'));
    });

    document.body.appendChild(overlay);
  }

  // Main flow
  fetchConfig(function(config) {
    if (!config) return; // No consent configured, exit silently

    // Update cookie name from config
    if (config.cookie_name) cookieName = config.cookie_name + '_' + SITE_ID;

    // Re-check cookie with correct name
    existing = getCookie(cookieName);
    parsed = existing ? parseConsent(existing) : null;

    if (parsed && parsed.c) {
      // Check if re-consent needed (version mismatch)
      if (parsed.v >= (config.version || 1)) {
        // Consent is valid and current
        setConsentState(parsed.c);
        return;
      }
      // Version changed, need re-consent — fall through to show banner
    }

    // Show banner and record the impression
    function showBanner(cfg) {
      renderBanner(cfg);
      recordConsent('show', {}, cfg.version || 1);
    }

    if (document.body) {
      showBanner(config);
    } else {
      document.addEventListener('DOMContentLoaded', function() { showBanner(config); });
    }
  });
})();
