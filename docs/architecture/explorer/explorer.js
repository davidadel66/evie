/* Evie system explorer viewer.
 *
 * One classic script, no modules, no build, no third-party code, so the page
 * works from file://. The top half is the viewer model: pure functions over
 * the merged explorer data (merging, view selection, layout, reference
 * resolution, address-bar state), exported for Node's test runner. The bottom
 * half renders the model to SVG and DOM in a browser.
 *
 * Data files call evieExplorer.manifest([...]) and evieExplorer.register({...});
 * see README.md beside this file for the format.
 */
(function (global) {
  "use strict";

  // ---------------------------------------------------------------- model

  var GROUPS = [
    { id: "memory", label: "Memory" },
    { id: "runtime", label: "Agent runtime" },
    { id: "intelligence", label: "Intelligence" },
    { id: "infrastructure", label: "Infrastructure" },
  ];

  // The explorer page sits at docs/architecture/explorer/, so the checkout's
  // root is three directories up.
  var CHECKOUT_ROOT = "../../../";

  var FACT_LISTS = ["packages", "imports", "commands", "routes", "databases", "glossary", "adrs"];
  var NODE_LISTS = ["does", "owns", "interfaces", "gotchas", "terms", "diagrams", "links", "contains", "alsoIn", "children"];
  var STATEMENT_LISTS = ["does", "owns", "interfaces", "gotchas"];

  function list(value) {
    return Array.isArray(value) ? value.slice() : [];
  }

  function byString(a, b) {
    return a < b ? -1 : a > b ? 1 : 0;
  }

  function groupIndex(id) {
    for (var i = 0; i < GROUPS.length; i++) {
      if (GROUPS[i].id === id) return i;
    }
    return -1;
  }

  function normalizeNode(node, owner) {
    var out = {};
    Object.keys(node).forEach(function (key) {
      out[key] = node[key];
    });
    NODE_LISTS.forEach(function (key) {
      out[key] = list(node[key]);
    });
    out.children = out.children.map(function (child) {
      return normalizeNode(child, owner);
    });
    out.group = node.group || "";
    out.subgroup = node.subgroup || "";
    out.owner = owner;
    return out;
  }

  function normalizeFacts(facts) {
    var out = {};
    Object.keys(facts || {}).forEach(function (key) {
      out[key] = facts[key];
    });
    FACT_LISTS.forEach(function (key) {
      out[key] = list(out[key]);
    });
    return out;
  }

  // mergeData combines the registered data objects into one model: the facts,
  // every content node indexed by id (children included), edges, flows, and
  // design proposals. It normalises missing lists and never validates; the
  // explorer tool's check does that.
  function mergeData(objects) {
    var data = { facts: normalizeFacts(null), nodes: [], nodeById: {}, edges: [], flows: [], proposals: [] };
    function index(node) {
      if (node.id && !data.nodeById[node.id]) data.nodeById[node.id] = node;
      node.children.forEach(index);
    }
    (objects || []).forEach(function (object) {
      if (!object || typeof object !== "object") return;
      if (object.kind === "facts") {
        data.facts = normalizeFacts(object);
      } else if (object.kind === "content") {
        list(object.nodes).forEach(function (raw) {
          var node = normalizeNode(raw, object.owner || "");
          data.nodes.push(node);
          index(node);
        });
        data.edges = data.edges.concat(list(object.edges));
        data.flows = data.flows.concat(list(object.flows));
      } else if (object.kind === "proposal") {
        data.proposals.push(object);
      }
    });
    data.packageByPath = {};
    data.facts.packages.forEach(function (p) {
      data.packageByPath[p.path] = p;
    });
    data.termByName = {};
    data.facts.glossary.forEach(function (t) {
      data.termByName[t.term] = t;
    });
    data.adrByNumber = {};
    data.facts.adrs.forEach(function (a) {
      data.adrByNumber[a.number] = a;
    });
    return data;
  }

  function packagePath(id) {
    return typeof id === "string" && id.indexOf("pkg:") === 0 ? id.slice(4) : null;
  }

  function byTitle(a, b) {
    return byString(a.title, b.title) || byString(a.id, b.id);
  }

  // groupOverview selects the component group overview: the four groups in
  // their fixed order, each with its subgroups and package nodes, and the
  // import edges between groups aggregated into counts.
  function groupOverview(data) {
    var groups = GROUPS.map(function (g) {
      return { id: g.id, label: g.label, subgroups: [] };
    });
    var groupOf = {};
    data.nodes.forEach(function (node) {
      var gi = groupIndex(node.group);
      if (node.level !== "component" || gi < 0) return;
      var path = packagePath(node.id);
      if (path) groupOf[path] = node.group;
      var subgroups = groups[gi].subgroups;
      var sub = null;
      for (var i = 0; i < subgroups.length; i++) {
        if (subgroups[i].label === node.subgroup) sub = subgroups[i];
      }
      if (!sub) {
        sub = { label: node.subgroup, nodes: [] };
        subgroups.push(sub);
      }
      sub.nodes.push({ id: node.id, title: node.title || node.id, path: path });
    });
    groups.forEach(function (g) {
      g.subgroups.sort(function (a, b) {
        return byString(a.label, b.label);
      });
      g.subgroups.forEach(function (s) {
        s.nodes.sort(byTitle);
      });
    });
    var counts = {};
    data.facts.imports.forEach(function (imp) {
      var from = groupOf[imp.from];
      var to = groupOf[imp.to];
      if (!from || !to || from === to) return;
      var key = from + ">" + to;
      counts[key] = (counts[key] || 0) + 1;
    });
    var links = Object.keys(counts).map(function (key) {
      var parts = key.split(">");
      return { from: parts[0], to: parts[1], count: counts[key] };
    });
    links.sort(function (a, b) {
      return groupIndex(a.from) - groupIndex(b.from) || groupIndex(a.to) - groupIndex(b.to);
    });
    return { groups: groups, links: links };
  }

  // Group overview geometry. Panels sit in a two-by-two grid chosen so the
  // most coupled pairs of groups are neighbours; panels in a row share a
  // height, so every arrow runs inside the gaps between panels.
  var OVERVIEW = {
    margin: 24,
    panelGap: 88,
    pad: 16,
    titleHeight: 44,
    subgroupHeight: 24,
    subgroupGap: 10,
    chipWidth: 168,
    chipHeight: 30,
    chipGap: 8,
    columns: 2,
    minHeight: 120,
    arrowOffset: 9,
    cornerInset: 30,
    labelAt: 0.3,
    grid: { intelligence: [0, 0], memory: [1, 0], runtime: [0, 1], infrastructure: [1, 1] },
  };

  function round(n) {
    return Math.round(n * 10) / 10;
  }

  function panelHeight(group) {
    var o = OVERVIEW;
    var h = o.titleHeight;
    group.subgroups.forEach(function (sub, i) {
      if (sub.label) h += o.subgroupHeight;
      var rows = Math.ceil(sub.nodes.length / o.columns);
      h += rows * o.chipHeight + Math.max(rows - 1, 0) * o.chipGap;
      if (i < group.subgroups.length - 1) h += o.subgroupGap;
    });
    return Math.max(h + o.pad, o.minHeight);
  }

  // layoutGroupOverview places the overview's panels, package chips, and
  // dependency arrows. Positions depend only on the overview, so the same
  // data always draws the same picture.
  function layoutGroupOverview(view) {
    var o = OVERVIEW;
    var panelWidth = 2 * o.pad + o.columns * o.chipWidth + (o.columns - 1) * o.chipGap;
    var rowHeights = [0, 0];
    view.groups.forEach(function (g) {
      var row = o.grid[g.id][1];
      rowHeights[row] = Math.max(rowHeights[row], panelHeight(g));
    });
    var panels = [];
    var nodes = [];
    view.groups.forEach(function (g) {
      var cell = o.grid[g.id];
      var panel = {
        id: g.id,
        label: g.label,
        count: 0,
        x: o.margin + cell[0] * (panelWidth + o.panelGap),
        y: o.margin + (cell[1] === 0 ? 0 : rowHeights[0] + o.panelGap),
        width: panelWidth,
        height: rowHeights[cell[1]],
        subgroups: [],
      };
      var cursor = panel.y + o.titleHeight;
      g.subgroups.forEach(function (sub, i) {
        if (sub.label) {
          panel.subgroups.push({ label: sub.label, x: panel.x + o.pad, y: cursor + 16 });
          cursor += o.subgroupHeight;
        }
        sub.nodes.forEach(function (n, k) {
          var col = k % o.columns;
          var row = Math.floor(k / o.columns);
          nodes.push({
            id: n.id,
            title: n.title,
            group: g.id,
            subgroup: sub.label,
            x: panel.x + o.pad + col * (o.chipWidth + o.chipGap),
            y: cursor + row * (o.chipHeight + o.chipGap),
            width: o.chipWidth,
            height: o.chipHeight,
          });
          panel.count++;
        });
        var rows = Math.ceil(sub.nodes.length / o.columns);
        cursor += rows * o.chipHeight + Math.max(rows - 1, 0) * o.chipGap;
        if (i < g.subgroups.length - 1) cursor += o.subgroupGap;
      });
      panels.push(panel);
    });
    var panelById = {};
    panels.forEach(function (p) {
      panelById[p.id] = p;
    });
    var edges = view.links.map(function (link) {
      return groupArrow(link, panelById[link.from], panelById[link.to]);
    });
    return {
      width: 2 * o.margin + 2 * panelWidth + o.panelGap,
      height: 2 * o.margin + rowHeights[0] + o.panelGap + rowHeights[1],
      panels: panels,
      nodes: nodes,
      edges: edges,
    };
  }

  // groupArrow draws one aggregated dependency from panel a to panel b. Two
  // directions between the same pair never share a line: neighbours offset
  // them either side of the gap's centre line, and diagonal neighbours leave
  // from the facing corner along a horizontal side one way and a vertical
  // side the other.
  function groupArrow(link, a, b) {
    var o = OVERVIEW;
    var ca = o.grid[a.id];
    var cb = o.grid[b.id];
    var forward = groupIndex(a.id) < groupIndex(b.id);
    var x1, y1, x2, y2;
    if (ca[1] === cb[1]) {
      var top = Math.max(a.y, b.y);
      var bottom = Math.min(a.y + a.height, b.y + b.height);
      y1 = y2 = (top + bottom) / 2 + (forward ? -o.arrowOffset : o.arrowOffset);
      x1 = ca[0] < cb[0] ? a.x + a.width : a.x;
      x2 = ca[0] < cb[0] ? b.x : b.x + b.width;
    } else if (ca[0] === cb[0]) {
      var left = Math.max(a.x, b.x);
      var right = Math.min(a.x + a.width, b.x + b.width);
      x1 = x2 = (left + right) / 2 + (forward ? -o.arrowOffset : o.arrowOffset);
      y1 = ca[1] < cb[1] ? a.y + a.height : a.y;
      y2 = ca[1] < cb[1] ? b.y : b.y + b.height;
    } else {
      var sx = cb[0] > ca[0] ? 1 : -1;
      var sy = cb[1] > ca[1] ? 1 : -1;
      var ax = sx > 0 ? a.x + a.width : a.x;
      var ay = sy > 0 ? a.y + a.height : a.y;
      var bx = sx > 0 ? b.x : b.x + b.width;
      var by = sy > 0 ? b.y : b.y + b.height;
      if (forward) {
        x1 = ax - sx * o.cornerInset;
        y1 = ay;
        x2 = bx + sx * o.cornerInset;
        y2 = by;
      } else {
        x1 = ax;
        y1 = ay - sy * o.cornerInset;
        x2 = bx;
        y2 = by + sy * o.cornerInset;
      }
    }
    return {
      from: link.from,
      to: link.to,
      count: link.count,
      x1: round(x1),
      y1: round(y1),
      x2: round(x2),
      y2: round(y2),
      labelX: round(x1 + (x2 - x1) * o.labelAt),
      labelY: round(y1 + (y2 - y1) * o.labelAt),
    };
  }

  function describePackage(data, path) {
    var id = "pkg:" + path;
    var node = data.nodeById[id];
    return { id: id, path: path, title: node && node.title ? node.title : path };
  }

  function sortedPaths(paths) {
    return paths.slice().sort(byString);
  }

  // packageDetails gathers what the panel shows for one node: its package
  // facts, callers and dependencies from the import graph, glossary terms
  // with definitions, and every extracted fact that names its package.
  function packageDetails(data, id) {
    var node = data.nodeById[id] || null;
    var path = packagePath(id);
    var callers = [];
    var dependencies = [];
    if (path) {
      data.facts.imports.forEach(function (imp) {
        if (imp.to === path) callers.push(imp.from);
        if (imp.from === path) dependencies.push(imp.to);
      });
    }
    return {
      node: node,
      path: path,
      pkg: path ? data.packageByPath[path] || null : null,
      callers: sortedPaths(callers).map(function (p) {
        return describePackage(data, p);
      }),
      dependencies: sortedPaths(dependencies).map(function (p) {
        return describePackage(data, p);
      }),
      terms: node ? nodeTerms(data, node) : [],
      facts: path ? factsForPackage(data.facts, path) : [],
    };
  }

  function nodeStatements(node) {
    var statements = node.summary ? [node.summary] : [];
    STATEMENT_LISTS.forEach(function (key) {
      statements = statements.concat(node[key]);
    });
    return statements;
  }

  function nodeTerms(data, node) {
    var names = node.terms.slice();
    nodeStatements(node).forEach(function (s) {
      list(s && s.refs).forEach(function (ref) {
        if (typeof ref === "string" && ref.indexOf("term:") === 0) {
          var term = ref.slice(5);
          if (names.indexOf(term) < 0) names.push(term);
        }
      });
    });
    return names.map(function (name) {
      var entry = data.termByName[name];
      return { term: name, definition: entry ? entry.definition : null, avoid: entry ? list(entry.avoid) : [] };
    });
  }

  var FACT_ORDER = ["commands", "routes", "databases"];
  var NOT_PACKAGE_FACTS = ["format", "kind", "packages", "imports", "glossary", "adrs"];

  function capitalize(word) {
    return word.charAt(0).toUpperCase() + word.slice(1);
  }

  function sourceLine(source) {
    return source && source.file ? [source.file + (source.function ? " " + source.function : "")] : [];
  }

  var FACT_FORMATS = {
    commands: function (c) {
      return [{ title: [c.binary].concat(list(c.words)).join(" "), detail: sourceLine(c.source) }];
    },
    routes: function (r) {
      var detail = r.guard ? ["guard " + r.guard] : [];
      return [{ title: (r.method ? r.method + " " : "") + r.pattern, detail: detail.concat(sourceLine(r.source)) }];
    },
    databases: function (db) {
      return list(db.tables).map(function (table) {
        var detail = list(table.columns).map(function (c) {
          return c.name + " " + c.type + (c.notNull ? " not null" : "") + (c.primaryKey ? ", primary key " + c.primaryKey : "");
        });
        list(table.foreignKeys).forEach(function (fk) {
          detail.push("(" + list(fk.columns).join(", ") + ") references " + fk.table + " (" + list(fk.references).join(", ") + ")");
        });
        return { title: db.id + "." + table.name, detail: detail };
      });
    },
  };

  var TITLE_FIELDS = ["name", "id", "title", "pattern"];

  function genericFact(item) {
    var titleField = null;
    for (var i = 0; i < TITLE_FIELDS.length && !titleField; i++) {
      if (typeof item[TITLE_FIELDS[i]] === "string") titleField = TITLE_FIELDS[i];
    }
    var detail = [];
    Object.keys(item).forEach(function (key) {
      var value = item[key];
      if (key === "package" || key === titleField) return;
      if (value === null || typeof value !== "object") detail.push(key + ": " + String(value));
    });
    return [{ title: titleField ? item[titleField] : "item", detail: detail }];
  }

  // factsForPackage renders every facts list whose items name the package:
  // known lists with their own format, any other list generically, so new
  // kinds of facts appear in the panel without a viewer change.
  function factsForPackage(facts, path) {
    var keys = Object.keys(facts).filter(function (key) {
      return NOT_PACKAGE_FACTS.indexOf(key) < 0 && Array.isArray(facts[key]);
    });
    keys.sort(function (a, b) {
      var ia = FACT_ORDER.indexOf(a);
      var ib = FACT_ORDER.indexOf(b);
      if (ia >= 0 || ib >= 0) return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib);
      return byString(a, b);
    });
    var sections = [];
    keys.forEach(function (key) {
      var format = FACT_FORMATS[key] || genericFact;
      var items = [];
      facts[key].forEach(function (item) {
        if (item && item.package === path) items = items.concat(format(item));
      });
      if (items.length) sections.push({ key: key, title: capitalize(key), items: items });
    });
    return sections;
  }

  // checkoutHref links a repository path into the owner's own checkout,
  // encoding each segment. A path that could leave the checkout or name a
  // scheme gets no link.
  function checkoutHref(path) {
    if (!path || path.charAt(0) === "/" || path.indexOf(":") >= 0 || path.indexOf("\\") >= 0) return null;
    var segments = path.split("/");
    for (var i = 0; i < segments.length; i++) {
      if (segments[i] === "" || segments[i] === "." || segments[i] === "..") return null;
    }
    return CHECKOUT_ROOT + segments.map(encodeURIComponent).join("/");
  }

  function splitAnchor(target) {
    var i = target.indexOf("#");
    return i < 0 ? { path: target, anchor: "" } : { path: target.slice(0, i), anchor: target.slice(i + 1) };
  }

  // resolveRef turns a typed reference into what the page shows: text, a
  // relative link into the checkout for code and doc references, a title
  // for ADRs and glossary terms, or an in-page target.
  function resolveRef(ref, data) {
    var i = typeof ref === "string" ? ref.indexOf(":") : -1;
    if (i <= 0) return { kind: "unknown", text: String(ref) };
    var kind = ref.slice(0, i);
    var target = ref.slice(i + 1);
    var out = { kind: kind, text: target };
    var href;
    switch (kind) {
      case "code":
        href = checkoutHref(splitAnchor(target).path);
        if (href) out.href = href;
        return out;
      case "doc":
        var parts = splitAnchor(target);
        href = checkoutHref(parts.path);
        if (href) out.href = href + (parts.anchor ? "#" + encodeURIComponent(parts.anchor) : "");
        return out;
      case "adr":
        var adr = data.adrByNumber[target];
        out.text = "ADR " + target + (adr ? ": " + adr.title : "");
        if (adr) {
          href = checkoutHref(adr.path);
          if (href) out.href = href;
          out.title = adr.title;
        }
        return out;
      case "term":
        var entry = data.termByName[target];
        if (entry) out.title = entry.definition;
        out.target = { type: "term", value: target };
        return out;
      case "pkg":
        out.target = { type: "node", value: "pkg:" + target };
        return out;
      case "node":
        var node = data.nodeById[target];
        if (node && node.title) out.text = node.title;
        out.target = { type: "node", value: target };
        return out;
      case "table":
      case "route":
      case "command":
        out.target = { type: kind, value: target };
        return out;
    }
    return { kind: "unknown", text: ref };
  }

  // The address fragment holds the current view and selected node.
  var HASH_KEYS = ["view", "node"];

  function formatHash(state) {
    var parts = [];
    HASH_KEYS.forEach(function (key) {
      if (state && state[key]) {
        parts.push(key + "=" + encodeURIComponent(state[key]).replace(/%3A/gi, ":").replace(/%2F/gi, "/"));
      }
    });
    return parts.length ? "#" + parts.join("&") : "";
  }

  function parseHash(hash) {
    var state = {};
    String(hash || "")
      .replace(/^#/, "")
      .split("&")
      .forEach(function (pair) {
        var i = pair.indexOf("=");
        if (i <= 0) return;
        var key = pair.slice(0, i);
        if (HASH_KEYS.indexOf(key) < 0) return;
        try {
          state[key] = decodeURIComponent(pair.slice(i + 1));
        } catch (e) {
          // a malformed escape leaves the key unset
        }
      });
    return state;
  }

  var model = {
    GROUPS: GROUPS,
    mergeData: mergeData,
    groupOverview: groupOverview,
    layoutGroupOverview: layoutGroupOverview,
    packageDetails: packageDetails,
    factsForPackage: factsForPackage,
    resolveRef: resolveRef,
    formatHash: formatHash,
    parseHash: parseHash,
  };

  if (typeof module !== "undefined" && module.exports) {
    module.exports = model;
  }

  if (typeof document === "undefined") return;

  // ------------------------------------------------------------- rendering

  var SVG_NS = "http://www.w3.org/2000/svg";

  function svg(name, attrs, parent) {
    var el = document.createElementNS(SVG_NS, name);
    Object.keys(attrs || {}).forEach(function (key) {
      el.setAttribute(key, String(attrs[key]));
    });
    if (parent) parent.appendChild(el);
    return el;
  }

  function html(name, attrs, parent, text) {
    var el = document.createElement(name);
    Object.keys(attrs || {}).forEach(function (key) {
      el.setAttribute(key, String(attrs[key]));
    });
    if (text !== undefined) el.textContent = text;
    if (parent) parent.appendChild(el);
    return el;
  }

  var app = { data: null, state: {}, missing: [] };

  function select(id) {
    app.state = { view: "components", node: id || "" };
    var hash = formatHash(app.state);
    try {
      history.replaceState(null, "", hash || location.pathname + location.search);
    } catch (e) {
      location.hash = hash;
    }
    render();
  }

  function renderOverview(canvas) {
    var data = app.data;
    var layout = layoutGroupOverview(groupOverview(data));
    canvas.textContent = "";
    var root = svg(
      "svg",
      {
        viewBox: "0 0 " + layout.width + " " + layout.height,
        class: "overview",
        role: "group",
        "aria-label": "Component group overview",
        preserveAspectRatio: "xMidYMin meet",
      },
      canvas
    );
    var defs = svg("defs", {}, root);
    var marker = svg(
      "marker",
      { id: "arrow", viewBox: "0 0 10 10", refX: 10, refY: 5, markerWidth: 7, markerHeight: 7, orient: "auto-start-reverse" },
      defs
    );
    svg("path", { d: "M0,0 L10,5 L0,10 z", class: "arrowhead" }, marker);

    layout.panels.forEach(function (p) {
      var g = svg("g", { class: "group-panel group-" + p.id }, root);
      svg("rect", { x: p.x, y: p.y, width: p.width, height: p.height, rx: 12 }, g);
      svg("text", { x: p.x + OVERVIEW.pad, y: p.y + 28, class: "panel-title" }, g).textContent = p.label;
      svg("text", { x: p.x + p.width - OVERVIEW.pad, y: p.y + 28, class: "panel-count", "text-anchor": "end" }, g).textContent =
        p.count + (p.count === 1 ? " package" : " packages");
      p.subgroups.forEach(function (s) {
        svg("text", { x: s.x, y: s.y, class: "subgroup-label" }, g).textContent = s.label;
      });
    });

    layout.edges.forEach(function (e) {
      var g = svg("g", { class: "dependency" }, root);
      svg("title", {}, g).textContent =
        groupLabel(e.from) + " imports " + groupLabel(e.to) + ": " + e.count + (e.count === 1 ? " import edge" : " import edges");
      svg("line", { x1: e.x1, y1: e.y1, x2: e.x2, y2: e.y2, "marker-end": "url(#arrow)" }, g);
      var text = String(e.count);
      var w = 10 + 7 * text.length;
      svg("rect", { x: e.labelX - w / 2, y: e.labelY - 9, width: w, height: 18, rx: 9, class: "count-pill" }, g);
      svg("text", { x: e.labelX, y: e.labelY + 4, "text-anchor": "middle", class: "count" }, g).textContent = text;
    });

    layout.nodes.forEach(function (n) {
      var path = packagePath(n.id);
      var pkg = path ? data.packageByPath[path] : null;
      var node = data.nodeById[n.id];
      var classes = ["chip", "group-" + n.group];
      if (pkg && pkg.command) classes.push("command");
      if (pkg && pkg.testSupport) classes.push("test-support");
      if (app.state.node === n.id) classes.push("selected");
      var g = svg(
        "g",
        { class: classes.join(" "), tabindex: 0, role: "button", "aria-label": "Package " + (path || n.title), "data-id": n.id },
        root
      );
      svg("title", {}, g).textContent = (path || n.title) + (node && node.summary ? ": " + node.summary.text : "");
      svg("rect", { x: n.x, y: n.y, width: n.width, height: n.height, rx: 7 }, g);
      svg("text", { x: n.x + 12, y: n.y + 20 }, g).textContent = n.title;
      g.addEventListener("click", function () {
        select(n.id);
      });
      g.addEventListener("keydown", function (event) {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          select(n.id);
        }
      });
    });
  }

  function groupLabel(id) {
    var g = GROUPS[groupIndex(id)];
    return g ? g.label : id;
  }

  function renderRefs(refs, parent) {
    var ul = html("ul", { class: "sources" }, parent);
    list(refs).forEach(function (ref) {
      var link = resolveRef(ref, app.data);
      var li = html("li", { class: "source source-" + link.kind }, ul);
      var el;
      if (link.href) {
        el = html("a", { href: link.href }, li, link.text);
      } else if (link.target && link.target.type === "node" && app.data.nodeById[link.target.value]) {
        el = html("button", { type: "button", class: "linklike" }, li, link.text);
        el.addEventListener("click", function () {
          select(link.target.value);
        });
      } else {
        el = html("span", {}, li, link.text);
      }
      if (link.title) el.setAttribute("title", link.title);
    });
  }

  function renderStatement(statement, parent) {
    if (!statement) return;
    var block = html("div", { class: "statement" }, parent);
    html("p", {}, block, statement.text || "");
    renderRefs(statement.refs, block);
  }

  function section(parent, title) {
    var s = html("section", {}, parent);
    html("h3", {}, s, title);
    return s;
  }

  function renderPackageList(parent, title, packages) {
    var s = section(parent, title + " (" + packages.length + ")");
    if (!packages.length) {
      html("p", { class: "empty" }, s, "None.");
      return;
    }
    var ul = html("ul", { class: "package-list" }, s);
    packages.forEach(function (p) {
      var li = html("li", {}, ul);
      var button = html("button", { type: "button", class: "package-button", title: p.path }, li, p.title);
      button.addEventListener("click", function () {
        select(p.id);
      });
    });
  }

  function renderPanel(panel) {
    var id = app.state.node;
    var details = id ? packageDetails(app.data, id) : null;
    panel.textContent = "";
    if (!details || !details.node) {
      panel.hidden = true;
      return;
    }
    panel.hidden = false;
    var node = details.node;
    var header = html("header", { class: "panel-header" }, panel);
    html("h2", {}, header, node.title || node.id);
    var close = html("button", { type: "button", class: "close", "aria-label": "Close panel" }, header, "×");
    close.addEventListener("click", function () {
      select("");
    });

    var meta = html("p", { class: "meta" }, panel);
    if (details.path) html("code", {}, meta, details.path);
    var tags = [];
    if (details.pkg) tags.push(details.pkg.command ? "command" : "library");
    if (details.pkg && details.pkg.testSupport) tags.push("test support");
    tags.push(groupLabel(node.group) + (node.subgroup ? " / " + node.subgroup : ""));
    tags.forEach(function (tag) {
      html("span", { class: "tag" }, meta, tag);
    });

    renderStatement(node.summary, section(panel, "Summary"));
    if (details.pkg && details.pkg.doc) {
      html("p", { class: "package-doc" }, section(panel, "Package doc"), details.pkg.doc);
    }
    STATEMENT_LISTS.forEach(function (key) {
      if (!node[key].length) return;
      var titles = { does: "What it does", owns: "Data it owns", interfaces: "Interfaces", gotchas: "Gotchas" };
      var s = section(panel, titles[key]);
      node[key].forEach(function (statement) {
        renderStatement(statement, s);
      });
    });
    renderPackageList(panel, "Who calls it", details.callers);
    renderPackageList(panel, "Depends on", details.dependencies);

    var termSection = section(panel, "Glossary terms");
    if (!details.terms.length) {
      html("p", { class: "empty" }, termSection, "None yet.");
    } else {
      var dl = html("dl", { class: "terms" }, termSection);
      details.terms.forEach(function (t) {
        html("dt", {}, dl, t.term);
        html("dd", {}, dl, t.definition || "Not in GLOSSARY.md.");
        if (t.avoid.length) html("dd", { class: "avoid" }, dl, "Avoid: " + t.avoid.join(", "));
      });
    }

    var factSection = section(panel, "Extracted facts");
    if (!details.facts.length) {
      html("p", { class: "empty" }, factSection, "No routes, commands, or tables extracted for this package.");
    }
    details.facts.forEach(function (group) {
      html("h4", {}, factSection, group.title + " (" + group.items.length + ")");
      var ul = html("ul", { class: "facts" }, factSection);
      group.items.forEach(function (item) {
        var li = html("li", {}, ul);
        html("code", {}, li, item.title);
        item.detail.forEach(function (line) {
          html("div", { class: "fact-detail" }, li, line);
        });
      });
    });

    if (node.links.length) renderRefs(node.links, section(panel, "Links"));
  }

  function renderRail(rail) {
    var listEl = rail.querySelector("#group-list");
    if (!listEl) return;
    listEl.textContent = "";
    groupOverview(app.data).groups.forEach(function (g) {
      var count = 0;
      g.subgroups.forEach(function (s) {
        count += s.nodes.length;
      });
      var li = html("li", { class: "group-" + g.id }, listEl);
      html("span", { class: "swatch", "aria-hidden": "true" }, li);
      html("span", {}, li, g.label);
      html("span", { class: "rail-count" }, li, String(count));
    });
  }

  function renderProblems() {
    var banner = document.getElementById("problems");
    if (!banner) return;
    banner.hidden = app.missing.length === 0;
    banner.textContent = app.missing.length ? "These data files did not load: " + app.missing.join(", ") : "";
  }

  function render() {
    renderProblems();
    renderRail(document.getElementById("rail"));
    renderOverview(document.getElementById("canvas"));
    renderPanel(document.getElementById("panel"));
  }

  function boot(objects, missing) {
    app.data = mergeData(objects);
    app.missing = missing;
    app.state = parseHash(location.hash);
    render();
    window.addEventListener("hashchange", function () {
      app.state = parseHash(location.hash);
      render();
    });
    document.addEventListener("keydown", function (event) {
      if (event.key === "Escape" && app.state.node) select("");
    });
  }

  // The loader. The manifest lists data files relative to data/; each is
  // loaded in order as a classic script, which registers one object.
  var registered = [];
  global.evieExplorer = {
    register: function (object) {
      registered.push(object);
    },
    manifest: function (files) {
      var missing = [];
      var i = 0;
      function next() {
        if (i >= files.length) {
          boot(registered, missing);
          return;
        }
        var file = files[i++];
        var script = document.createElement("script");
        script.src = "data/" + file;
        script.onload = next;
        script.onerror = function () {
          missing.push(file);
          next();
        };
        document.head.appendChild(script);
      }
      next();
    },
  };
})(typeof window !== "undefined" ? window : globalThis);
