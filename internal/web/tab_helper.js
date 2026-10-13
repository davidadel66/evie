// Evie's helper for a Workspace tab (workspace-tabs.spec.md, story 2).
// A tab loads it with <script src="_evie/tab.js"></script> and then reads
// only what its tab.json declares and the owner approved:
//   await evie.query("finance", "SELECT ...")  -> {columns, rows, truncated}
//   await evie.file("eval/run-1/metrics.json") -> the file's text
//   await evie.runs()                          -> this Workspace's runs
// It greets Evie's page once with the key Evie put in this page's address
// and talks only over the private channel Evie returns, so a page this frame
// is navigated to later cannot use the tab's access. It also tells Evie what
// goes wrong on the page (script errors, anything the page's policy blocks),
// so Evie can fix the tab it wrote. Load it first, in <head>.
(() => {
  const key = new URLSearchParams(location.hash.slice(1)).get('evie-key') || '';
  const waiting = new Map();
  let next = 0;
  // Problems wait here until the channel is open; at most 20 per page.
  const problems = [];
  let channel;
  let reported = 0;
  const flush = () => {
    while (channel && problems.length) channel.postMessage({ problem: problems.shift() });
  };
  const report = (kind, message) => {
    if (reported++ >= 20) return;
    problems.push({ kind, message: String(message).slice(0, 500) });
    flush();
  };
  addEventListener('error', event => {
    const target = event.target;
    if (target && target !== window && (target.src || target.href)) report('error', `${target.tagName.toLowerCase()} ${target.src || target.href} failed to load`);
    else report('error', `${event.message || 'error'} (${(event.filename || '').split('/').pop() || 'index.html'}:${event.lineno || 0})`);
  }, true);
  addEventListener('unhandledrejection', event => report('error', `unhandled: ${(event.reason && event.reason.message) || event.reason}`));
  document.addEventListener('securitypolicyviolation', event => report('blocked', `${event.effectiveDirective} blocked ${event.blockedURI || 'inline code'}`));
  const connected = new Promise((resolve, reject) => {
    if (window.parent === window) {
      reject(new Error('Open this tab in Evie to read its data.'));
      return;
    }
    const greeted = event => {
      if (event.source !== window.parent || !event.data || event.data.evie !== 'tab-port' || !event.ports[0]) return;
      removeEventListener('message', greeted);
      const port = event.ports[0];
      port.onmessage = ({ data }) => {
        const pending = waiting.get(data && data.id);
        if (!pending) return;
        waiting.delete(data.id);
        if (data.ok) pending.resolve(data.result);
        else pending.reject(new Error(data.error || 'Evie refused the request.'));
      };
      channel = port;
      flush();
      resolve(port);
    };
    addEventListener('message', greeted);
    window.parent.postMessage({ evie: 'tab-hello', key }, '*');
  });
  const ask = async request => {
    const port = await connected;
    return new Promise((resolve, reject) => {
      const id = ++next;
      waiting.set(id, { resolve, reject });
      port.postMessage({ id, request });
    });
  };
  window.evie = Object.freeze({
    query: (db, sql) => ask({ kind: 'query', db, sql }),
    file: path => ask({ kind: 'file', path }),
    runs: () => ask({ kind: 'runs' }),
  });
})();
