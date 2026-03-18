/**
 * Etiquetta Session Recorder
 * Privacy-first session replay using rrweb.
 * Loaded conditionally by tracker.js when replay is enabled.
 */
(function() {
  "use strict";

  if (window.__ETIQUETTA_RECORDER_LOADED__) return;
  window.__ETIQUETTA_RECORDER_LOADED__ = true;

  var SCRIPT = (function() {
    var scripts = document.querySelectorAll('script[src*="s.js"]');
    for (var i = 0; i < scripts.length; i++) {
      var src = scripts[i].src;
      if (src && src.includes('s.js')) {
        return new URL(src).origin;
      }
    }
    return location.origin;
  })();

  var BASE_URL = SCRIPT;
  var SESSION_ID = null;
  var VISITOR_HASH = window.etiquetta ? window.etiquetta.getVisitorHash() : '';
  var CONFIG = null;
  var eventBuffer = [];
  var FLUSH_INTERVAL = 10000; // 10 seconds
  var MAX_BUFFER = 50;
  var flushTimer = null;
  var recordingStartTime = null;
  var stopFn = null;
  var metaSent = false;

  function log() {
    if (window.__ETIQUETTA_CONFIG__ && window.__ETIQUETTA_CONFIG__.debug) {
      console.log.apply(console, ['[Etiquetta Recorder]'].concat(Array.prototype.slice.call(arguments)));
    }
  }

  function uuid() {
    if (crypto && crypto.randomUUID) return crypto.randomUUID().replace(/-/g, '');
    return 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'.replace(/x/g, function() {
      return ((Math.random() * 16) | 0).toString(16);
    });
  }

  // Get device type
  function getDeviceType() {
    var ua = navigator.userAgent;
    if (/Mobi|Android/i.test(ua)) return 'mobile';
    if (/Tablet|iPad/i.test(ua)) return 'tablet';
    return 'desktop';
  }

  // Get browser name
  function getBrowserName() {
    var ua = navigator.userAgent;
    if (/Firefox\//.test(ua)) return 'Firefox';
    if (/Edg\//.test(ua)) return 'Edge';
    if (/Chrome\//.test(ua)) return 'Chrome';
    if (/Safari\//.test(ua)) return 'Safari';
    if (/Opera|OPR\//.test(ua)) return 'Opera';
    return 'Other';
  }

  // Get OS name
  function getOSName() {
    var ua = navigator.userAgent;
    if (/Windows/.test(ua)) return 'Windows';
    if (/Mac OS/.test(ua)) return 'macOS';
    if (/Linux/.test(ua)) return 'Linux';
    if (/Android/.test(ua)) return 'Android';
    if (/iOS|iPhone|iPad/.test(ua)) return 'iOS';
    return 'Other';
  }

  // Flush buffered events to server
  function flush() {
    if (flushTimer) {
      clearTimeout(flushTimer);
      flushTimer = null;
    }
    if (!eventBuffer.length || !SESSION_ID) return;

    var events = eventBuffer.splice(0, eventBuffer.length);
    var payload = {
      session_id: SESSION_ID,
      domain: location.hostname,
      visitor_hash: VISITOR_HASH,
      events: events
    };

    // Include meta on first flush
    if (!metaSent) {
      metaSent = true;
      payload.meta = {
        url: location.href,
        device_type: getDeviceType(),
        browser_name: getBrowserName(),
        os_name: getOSName(),
        geo_country: '',
        screen_width: screen.width || 0,
        screen_height: screen.height || 0
      };
    }

    var body = JSON.stringify(payload);

    if (navigator.sendBeacon) {
      navigator.sendBeacon(BASE_URL + '/r', new Blob([body], { type: 'application/json' }));
    } else {
      fetch(BASE_URL + '/r', {
        method: 'POST',
        body: body,
        keepalive: true,
        headers: { 'Content-Type': 'application/json' }
      }).catch(function() {});
    }

    log('Flushed', events.length, 'replay events');
  }

  function scheduleFlush() {
    if (!flushTimer) {
      flushTimer = setTimeout(flush, FLUSH_INTERVAL);
    }
  }

  // Sampling: deterministic based on visitor hash
  function shouldRecord(sampleRate) {
    if (sampleRate >= 100) return true;
    if (sampleRate <= 0) return false;
    // Use visitor hash for deterministic sampling
    var hash = 0;
    var str = VISITOR_HASH || String(Math.random());
    for (var i = 0; i < str.length; i++) {
      hash = ((hash << 5) - hash + str.charCodeAt(i)) | 0;
    }
    return (Math.abs(hash) % 100) < sampleRate;
  }

  // Start recording using rrweb
  function startRecording(config) {
    if (!window.rrweb || !window.rrweb.record) {
      log('rrweb not loaded, cannot record');
      return;
    }

    SESSION_ID = uuid();
    recordingStartTime = Date.now();
    var maxDuration = (config.max_duration_sec || 1800) * 1000;

    log('Starting recording, session:', SESSION_ID, 'max duration:', maxDuration / 1000, 's');

    stopFn = window.rrweb.record({
      emit: function(event) {
        // Check max duration
        if (Date.now() - recordingStartTime > maxDuration) {
          stopRecording();
          return;
        }

        eventBuffer.push(event);

        if (eventBuffer.length >= MAX_BUFFER) {
          flush();
        } else {
          scheduleFlush();
        }
      },
      maskAllText: config.mask_text !== false,
      maskAllInputs: config.mask_inputs !== false,
      blockSelector: '.etq-no-record',
      maskTextSelector: '.etq-mask',
      inlineStylesheet: true,
      sampling: {
        mousemove: true,
        mouseInteraction: true,
        scroll: 150,
        media: 800,
        input: 'last'
      }
    });

    // Flush on page hide / unload
    document.addEventListener('visibilitychange', function() {
      if (document.visibilityState === 'hidden') flush();
    });
    window.addEventListener('pagehide', flush);
    window.addEventListener('beforeunload', flush);
  }

  function stopRecording() {
    if (stopFn) {
      stopFn();
      stopFn = null;
    }
    flush();
    log('Recording stopped');
  }

  // Load rrweb from CDN (MIT licensed, same lib PostHog/Highlight use)
  function loadRrweb(callback) {
    if (window.rrweb && window.rrweb.record) {
      callback();
      return;
    }

    var script = document.createElement('script');
    script.src = 'https://cdn.jsdelivr.net/npm/rrweb@2.0.0-alpha.17/dist/rrweb.umd.cjs.js';
    script.crossOrigin = 'anonymous';
    script.onload = function() {
      log('rrweb loaded');
      callback();
    };
    script.onerror = function() {
      log('Failed to load rrweb');
    };
    document.head.appendChild(script);
  }

  // Initialize: fetch config, check sampling, load rrweb, start recording
  function init() {
    // Check consent
    var consent = window.__ETIQUETTA_CONSENT__;
    if (consent && consent.analytics === false) {
      log('Analytics consent not granted, skipping replay');
      return;
    }

    // Respect DNT/GPC
    var etqConfig = window.__ETIQUETTA_CONFIG__ || {};
    if (etqConfig.respectDNT !== false) {
      if (navigator.doNotTrack === '1' || navigator.globalPrivacyControl === true) {
        log('DNT/GPC detected, skipping replay');
        return;
      }
    }

    // Fetch replay config
    fetch(BASE_URL + '/r/config')
      .then(function(res) { return res.json(); })
      .then(function(config) {
        if (!config.enabled) {
          log('Replay disabled on server');
          return;
        }

        CONFIG = config;

        if (!shouldRecord(config.sample_rate || 10)) {
          log('Session not sampled for recording (rate:', config.sample_rate + '%)');
          return;
        }

        log('Session selected for recording');
        loadRrweb(function() {
          startRecording(config);
        });
      })
      .catch(function(err) {
        log('Failed to fetch replay config:', err);
      });
  }

  // Wait for tracker to initialize
  if (window.etiquetta) {
    VISITOR_HASH = window.etiquetta.getVisitorHash();
    init();
  } else {
    // Tracker might not be ready yet, wait a bit
    var checkInterval = setInterval(function() {
      if (window.etiquetta) {
        clearInterval(checkInterval);
        VISITOR_HASH = window.etiquetta.getVisitorHash();
        init();
      }
    }, 100);
    // Give up after 5 seconds
    setTimeout(function() { clearInterval(checkInterval); }, 5000);
  }
})();
