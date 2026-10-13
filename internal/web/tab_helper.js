// Evie's helper for a Workspace tab (workspace-tabs.spec.md, story 2).
// A tab loads it with <script src="_evie/tab.js"></script> and then reads
// only what its tab.json declares and the owner approved:
//   await evie.query("finance", "SELECT ...")  -> {columns, rows, truncated},
//                                                 rows as objects by column name
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
  // The helper makes the private channel and sends its far end with the
  // greeting, so the channel stays with this page. It greets again until
  // Evie answers, in case Evie's page was not listening yet.
  const connected = new Promise((resolve, reject) => {
    if (window.parent === window) {
      reject(new Error('Open this tab in Evie to read its data.'));
      return;
    }
    let attempts = 0;
    let done = false;
    const greet = () => {
      if (done) return;
      if (attempts++ === 20) {
        reject(new Error('Evie did not answer this tab; reload it.'));
        return;
      }
      const { port1, port2 } = new MessageChannel();
      port1.onmessage = ({ data }) => {
        if (!done && data && data.ready) {
          done = true;
          port1.onmessage = ({ data: reply }) => {
            const pending = waiting.get(reply && reply.id);
            if (!pending) return;
            waiting.delete(reply.id);
            if (reply.ok) pending.resolve(pending.shape ? pending.shape(reply.result) : reply.result);
            else pending.reject(new Error(reply.error || 'Evie refused the request.'));
          };
          channel = port1;
          flush();
          resolve(port1);
        }
      };
      window.parent.postMessage({ evie: 'tab-hello', key }, '*', [port2]);
      setTimeout(greet, 300);
    };
    greet();
  });
  const ask = async (request, shape) => {
    const port = await connected;
    return new Promise((resolve, reject) => {
      const id = ++next;
      waiting.set(id, { resolve, reject, shape });
      port.postMessage({ id, request });
    });
  };
  // A query's rows arrive as arrays in column order; a tab gets objects
  // keyed by column name, with columns still giving the order.
  const byColumn = result => result && Array.isArray(result.columns) && Array.isArray(result.rows)
    ? { ...result, rows: result.rows.map(row => Object.fromEntries(result.columns.map((column, i) => [column, row[i]]))) }
    : result;
  window.evie = Object.freeze({
    query: (db, sql) => ask({ kind: 'query', db, sql }, byColumn),
    file: path => ask({ kind: 'file', path }),
    runs: () => ask({ kind: 'runs' }),
  });
})();
