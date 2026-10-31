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

  // Served by evie serve, the page links to GitHub instead
  // (system-page.spec.md). build.js names the repository as an https URL of
  // plain path segments. COMMIT is a commit SHA, the build's or one a
  // History entry names.
  var REPOSITORY_URL = /^https:\/\/[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+(\/[A-Za-z0-9_][A-Za-z0-9._-]*)+$/;
  var COMMIT = /^[0-9a-f]{7,40}$/;

  // DEFAULT_REPOSITORY is the public repository. History's commits, tickets,
  // and pull requests live only on GitHub, so they link there from disk too,
  // and served pages link there when build.js names no usable repository.
  var DEFAULT_REPOSITORY = "https://github.com/davidadel66/evie";

  // DISK_LINKS are where a page opened from disk links (pageLinks), and
  // where links go when a caller gives none.
  var DISK_LINKS = { root: CHECKOUT_ROOT, repository: DEFAULT_REPOSITORY };

  var FACT_LISTS = ["packages", "imports", "commands", "routes", "databases", "glossary", "adrs", "history"];
  var NODE_LISTS = ["does", "owns", "interfaces", "gotchas", "terms", "diagrams", "links", "contains", "alsoIn", "children"];
  var STATEMENT_LISTS = ["does", "owns", "interfaces", "gotchas"];

  function list(value) {
    return Array.isArray(value) ? value.slice() : [];
  }

  function byString(a, b) {
    return a < b ? -1 : a > b ? 1 : 0;
  }

  // dict returns a map with no prototype. Every map keyed by data (node ids,
  // terms, paths, proposal ids) is one, so a key such as "constructor" or
  // "__proto__" finds only what the data put there.
  function dict() {
    return Object.create(null);
  }

  // own reads a fixed lookup table by a key that came from data, ignoring
  // what the table inherits.
  function own(table, key) {
    return Object.prototype.hasOwnProperty.call(table, key) ? table[key] : undefined;
  }

  // copy shallow-copies object's own keys into into, or into a new plain
  // object, and returns the result.
  function copy(object, into) {
    var out = into || {};
    Object.keys(object || {}).forEach(function (key) {
      out[key] = object[key];
    });
    return out;
  }

  function groupIndex(id) {
    for (var i = 0; i < GROUPS.length; i++) {
      if (GROUPS[i].id === id) return i;
    }
    return -1;
  }

  function groupLabel(id) {
    var g = GROUPS[groupIndex(id)];
    return g ? g.label : id;
  }

  function normalizeNode(node, owner) {
    var out = copy(node);
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
    var out = copy(facts);
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
    var data = { facts: normalizeFacts(null), nodes: [], nodeById: dict(), edges: [], flows: [], proposals: [] };
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
    data.packageByPath = dict();
    data.facts.packages.forEach(function (p) {
      data.packageByPath[p.path] = p;
    });
    data.termByName = dict();
    data.facts.glossary.forEach(function (t) {
      data.termByName[t.term] = t;
    });
    data.adrByNumber = dict();
    data.facts.adrs.forEach(function (a) {
      data.adrByNumber[a.number] = a;
    });
    data.links = buildLinks(data);
    return data;
  }

  function linkKey(edge) {
    return edge.from + "\u0000" + edge.to + "\u0000" + edge.via;
  }

  // buildLinks lists every connection the views draw: the facts' import
  // edges between package nodes, then the content edges. A content edge with
  // via import labels the facts edge it matches instead of repeating it.
  function buildLinks(data) {
    var links = [];
    var byKey = dict();
    data.facts.imports.forEach(function (imp) {
      var link = { from: "pkg:" + imp.from, to: "pkg:" + imp.to, via: "import", label: "", refs: [] };
      if (byKey[linkKey(link)]) return;
      byKey[linkKey(link)] = link;
      links.push(link);
    });
    data.edges.forEach(function (edge) {
      var key = linkKey(edge);
      if (byKey[key]) {
        if (edge.label) byKey[key].label = edge.label;
        byKey[key].refs = byKey[key].refs.concat(list(edge.refs));
        return;
      }
      var link = { from: edge.from, to: edge.to, via: edge.via, label: edge.label || "", refs: list(edge.refs) };
      byKey[key] = link;
      links.push(link);
    });
    return links;
  }

  // Proposal operations name the item beside "op"; these keys identify the
  // item rather than change it.
  var IDENTITY_KEYS = { node: ["op", "id"], edge: ["op", "from", "to", "via"], flow: ["op", "id"] };

  function changedKeys(op, what) {
    return Object.keys(op)
      .filter(function (key) {
        return IDENTITY_KEYS[what].indexOf(key) < 0;
      })
      .sort(byString);
  }

  // applyOp applies one add, change, or remove operation to a list of items
  // identified by keyOf, and returns the new list. normalize shapes an added
  // item; listKeys names the fields that are lists.
  function applyOp(items, op, what, proposalId, keyOf, normalize, listKeys) {
    var key = keyOf(op);
    var at = -1;
    for (var i = 0; i < items.length; i++) {
      if (keyOf(items[i]) === key) at = i;
    }
    var existing = at >= 0 ? items[at] : null;
    var item;
    if (op.op === "add" && !existing) {
      var raw = copy(op);
      delete raw.op;
      item = normalize(raw);
      item.change = "added";
      item.proposal = proposalId;
      return items.concat([item]);
    }
    if (!existing) return items;
    item = copy(existing);
    item.proposal = proposalId;
    if (op.op === "change") {
      var keys = changedKeys(op, what);
      keys.forEach(function (k) {
        item[k] = listKeys.indexOf(k) >= 0 ? list(op[k]) : op[k];
      });
      if (existing.change !== "added") {
        item.change = "changed";
        item.was = existing.was || existing;
        item.changed = list(existing.changed)
          .concat(keys)
          .filter(function (k, j, all) {
            return all.indexOf(k) === j;
          })
          .sort(byString);
      }
    } else if (op.op === "remove") {
      item.change = "removed";
    } else {
      return items;
    }
    var out = items.slice();
    out[at] = item;
    return out;
  }

  function flowKey(flow) {
    return flow.id;
  }

  function nodeKey(node) {
    return node.id;
  }

  function normalizeLink(edge) {
    var out = copy(edge);
    out.label = edge.label || "";
    out.refs = list(edge.refs);
    return out;
  }

  function normalizeFlow(flow) {
    var out = copy(flow);
    out.participants = list(flow.participants);
    out.steps = list(flow.steps);
    return out;
  }

  // applyProposals returns the data as the active design proposals would
  // leave it, without touching the built data: added items carry change
  // "added", changed items carry change "changed" with the changed field
  // names and the item as built in "was", and removed items stay in place
  // with change "removed" so the page can strike them through. An edge whose
  // node is removed is removed with it. Proposals apply in the order the data
  // lists them; unknown ids are ignored.
  function applyProposals(data, ids) {
    var wanted = list(ids);
    var out = copy(data);
    var nodes = data.nodes.slice();
    var links = data.links.slice();
    var flows = data.flows.slice();
    out.active = [];
    data.proposals.forEach(function (proposal) {
      if (wanted.indexOf(proposal.id) < 0 || out.active.indexOf(proposal.id) >= 0) return;
      out.active.push(proposal.id);
      list(proposal.nodes).forEach(function (op) {
        nodes = applyOp(nodes, op, "node", proposal.id, nodeKey, function (raw) {
          return normalizeNode(raw, "proposal:" + proposal.id);
        }, NODE_LISTS);
      });
      list(proposal.edges).forEach(function (op) {
        links = applyOp(links, op, "edge", proposal.id, linkKey, normalizeLink, ["refs"]);
      });
      list(proposal.flows).forEach(function (op) {
        flows = applyOp(flows, op, "flow", proposal.id, flowKey, normalizeFlow, ["participants", "steps"]);
      });
    });
    out.nodes = nodes;
    out.nodeById = copy(data.nodeById, dict());
    nodes.forEach(function (node) {
      out.nodeById[node.id] = node;
    });
    out.links = links.map(function (link) {
      var gone = [link.from, link.to].some(function (id) {
        return out.nodeById[id] && out.nodeById[id].change === "removed";
      });
      if (!gone || link.change === "removed" || link.change === "added") return link;
      var removed = copy(link);
      removed.change = "removed";
      return removed;
    });
    out.flows = flows;
    return out;
  }

  // How the page draws each kind of change a design proposal makes.
  var CHANGE_STYLES = {
    added: { label: "Added", dashed: true, badge: "", struck: false, faded: false, legend: "Added by a design proposal (dashed outline)" },
    changed: { label: "Changed", dashed: false, badge: "Δ", struck: false, faded: false, legend: "Changed by a design proposal (Δ badge)" },
    removed: { label: "Removed", dashed: false, badge: "", struck: true, faded: true, legend: "Removed by a design proposal (struck through)" },
  };
  var CHANGES = ["added", "changed", "removed"];

  function changeStyle(change) {
    var style = own(CHANGE_STYLES, change);
    if (!style) return null;
    return {
      className: "change-" + change,
      label: style.label,
      dashed: style.dashed,
      badge: style.badge,
      struck: style.struck,
      faded: style.faded,
    };
  }

  function proposalLegend() {
    return CHANGES.map(function (change) {
      return { change: change, className: "change-" + change, text: CHANGE_STYLES[change].legend };
    });
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
    var groupOf = dict();
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
      sub.nodes.push({ id: node.id, title: node.title || node.id, path: path, change: node.change || "" });
    });
    groups.forEach(function (g) {
      g.subgroups.sort(function (a, b) {
        return byString(a.label, b.label);
      });
      g.subgroups.forEach(function (s) {
        s.nodes.sort(byTitle);
      });
    });
    var counts = dict();
    data.links.forEach(function (link) {
      if (link.via !== "import" || link.change === "removed") return;
      var from = groupOf[packagePath(link.from)];
      var to = groupOf[packagePath(link.to)];
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
            ref: n.id,
            title: n.title,
            group: g.id,
            subgroup: sub.label,
            change: n.change || "",
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
    var panelById = dict();
    panels.forEach(function (p) {
      panelById[p.id] = p;
    });
    var edges = view.links.map(function (link) {
      return groupArrow(link, panelById[link.from], panelById[link.to]);
    });
    return {
      kind: "overview",
      width: 2 * o.margin + 2 * panelWidth + o.panelGap,
      height: 2 * o.margin + rowHeights[0] + o.panelGap + rowHeights[1],
      panels: panels,
      nodes: nodes,
      edges: edges,
      lanes: [],
      empty: nodes.length ? null : { title: "No packages yet", text: "Packages appear once content files give their nodes a group." },
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

  // ----------------------------------------------------------- layout kit
  //
  // Every view's layout returns the same shape: {kind, width, height, nodes,
  // edges, lanes, empty}. A drawn node has an id unique in its layout, the
  // explorer node it stands for in ref (or null), a title, a box (x, y,
  // width, height), and the change a proposal makes to it. A drawn edge joins
  // two drawn nodes, starting at (x1, y1) on the first and ending at (x2, y2)
  // on the second, and carries its SVG path in d.

  // Text is measured by estimate, so layouts stay pure and deterministic.
  var NARROW_CHARS = "iljtfr.,:;'|!()[]{} ";
  var WIDE_CHARS = "mwMW@%";

  function textWidth(text, size) {
    var w = 0;
    var s = String(text == null ? "" : text);
    for (var i = 0; i < s.length; i++) {
      var ch = s.charAt(i);
      if (NARROW_CHARS.indexOf(ch) >= 0) w += 0.3;
      else if (WIDE_CHARS.indexOf(ch) >= 0) w += 0.86;
      else if (ch >= "A" && ch <= "Z") w += 0.66;
      else if (ch >= "0" && ch <= "9") w += 0.58;
      else w += 0.54;
    }
    return round(w * (size || 13));
  }

  // monoWidth measures text set in a monospace face.
  function monoWidth(text, size) {
    return round(String(text == null ? "" : text).length * 0.62 * (size || 13));
  }

  // wrapText breaks text into lines no wider than maxWidth, at spaces where
  // it can; a word too long for a line is cut. At most maxLines lines are
  // kept, the last ending in an ellipsis when text was dropped.
  function wrapText(text, maxWidth, size, maxLines) {
    var words = String(text == null ? "" : text).split(/\s+/).filter(Boolean);
    var lines = [];
    var line = "";
    function push(word) {
      while (textWidth(word, size) > maxWidth && word.length > 1) {
        var cut = word.length - 1;
        while (cut > 1 && textWidth(word.slice(0, cut), size) > maxWidth) cut--;
        lines.push(word.slice(0, cut));
        word = word.slice(cut);
      }
      return word;
    }
    words.forEach(function (word) {
      var next = line ? line + " " + word : word;
      if (textWidth(next, size) <= maxWidth) {
        line = next;
        return;
      }
      if (line) lines.push(line);
      line = push(word);
    });
    if (line) lines.push(line);
    if (maxLines && lines.length > maxLines) {
      lines = lines.slice(0, maxLines);
      var last = lines[maxLines - 1];
      while (last.length > 1 && textWidth(last + "…", size) > maxWidth) last = last.slice(0, -1);
      lines[maxLines - 1] = last.replace(/\s+$/, "") + "…";
    }
    return lines;
  }

  var KIND_LABELS = {
    person: "Person",
    system: "Software system",
    "external-system": "External system",
    process: "Process",
    "user-interface": "User interface",
    cli: "Command-line tool",
    store: "Data store",
    package: "Package",
  };

  function kindLabel(kind) {
    return own(KIND_LABELS, kind) || kind || "";
  }

  // drawnNode makes a layout node for an explorer node.
  function drawnNode(node, width, height, extra) {
    var out = {
      id: node.id,
      ref: node.id,
      title: node.title || node.id,
      subtitle: kindLabel(node.kind),
      kind: node.kind || "",
      group: node.group || "",
      change: node.change || "",
      x: 0,
      y: 0,
      width: round(width),
      height: round(height),
    };
    return copy(extra, out);
  }

  function emptyLayout(kind, title, text) {
    return { kind: kind, width: 0, height: 0, nodes: [], edges: [], lanes: [], empty: { title: title, text: text } };
  }

  // separateLabels moves edge labels apart. In reading order, each label
  // takes the nearest free spot around where its edge puts it, clear of the
  // labels already placed and of the nodes; failing that, it moves down
  // below the labels it would cover.
  function separateLabels(edges, nodes) {
    var boxes = [];
    edges.forEach(function (e) {
      var lines = list(e.labelLines).length;
      if (!lines) return;
      var h = lines * LABEL_LINE + 6;
      boxes.push({ e: e, x: e.labelX - e.labelWidth / 2, y: e.labelY - h / 2, w: e.labelWidth, h: h });
    });
    boxes.sort(function (a, b) {
      return a.y - b.y || a.x - b.x || byString(a.e.id, b.e.id);
    });
    var obstacles = list(nodes)
      .filter(function (n) {
        return n.shape !== "initial";
      })
      .map(function (n) {
        return { x: n.x, y: n.y, w: n.width, h: n.height };
      });
    var placed = [];
    function hits(b, x, y, among) {
      for (var i = 0; i < among.length; i++) {
        var p = among[i];
        if (x < p.x + p.w + 2 && p.x < x + b.w + 2 && y < p.y + p.h + 2 && p.y < y + b.h + 2) return p;
      }
      return null;
    }
    boxes.forEach(function (b) {
      var dy = b.h + 3;
      var dx = b.w / 2 + 12;
      var tries = [[0, 0], [0, dy], [0, -dy], [dx, 0], [-dx, 0], [0, 2 * dy], [0, -2 * dy], [dx, dy], [-dx, dy], [dx, -dy], [-dx, -dy], [0, 3 * dy], [0, -3 * dy], [2 * dx, 0], [-2 * dx, 0]];
      var spot = null;
      for (var t = 0; t < tries.length && !spot; t++) {
        var x = b.x + tries[t][0];
        var y = b.y + tries[t][1];
        if (!hits(b, x, y, placed) && !hits(b, x, y, obstacles)) spot = [x, y];
      }
      if (spot) {
        b.x = spot[0];
        b.y = spot[1];
      } else {
        for (var guard = 0; guard <= boxes.length; guard++) {
          var hit = hits(b, b.x, b.y, placed);
          if (!hit) break;
          b.y = hit.y + hit.h + 3;
        }
      }
      placed.push(b);
      b.e.labelX = round(b.x + b.w / 2);
      b.e.labelY = round(b.y + b.h / 2);
    });
  }

  // finishLayout gives the layout its extent and rounds every box.
  function finishLayout(layout, margin) {
    separateLabels(layout.edges || [], layout.nodes);
    var w = 0;
    var h = 0;
    layout.nodes.forEach(function (n) {
      n.x = round(n.x);
      n.y = round(n.y);
      w = Math.max(w, n.x + n.width);
      h = Math.max(h, n.y + n.height);
    });
    (layout.lanes || []).forEach(function (l) {
      w = Math.max(w, l.x + l.width);
      h = Math.max(h, l.y + l.height);
    });
    (layout.edges || []).forEach(function (e) {
      if (e.labelX != null) {
        w = Math.max(w, e.labelX + (e.labelWidth || 0) / 2);
        h = Math.max(h, e.labelY + (list(e.labelLines).length * LABEL_LINE) / 2 + 6);
      }
      (e.extent || []).forEach(function (p) {
        w = Math.max(w, p.x);
        h = Math.max(h, p.y);
      });
      delete e.extent;
    });
    layout.width = round(w + margin);
    layout.height = round(h + margin);
    if (!layout.lanes) layout.lanes = [];
    if (layout.empty === undefined) layout.empty = null;
    return layout;
  }

  var NORMALS = { left: [-1, 0], right: [1, 0], top: [0, -1], bottom: [0, 1] };

  // facingSides chooses where an edge between two boxes leaves and arrives:
  // side to side when they sit apart horizontally, top to bottom when they
  // sit apart vertically.
  function facingSides(a, b) {
    if (a === b) return ["right", "right"];
    if (a.x + a.width <= b.x) return ["right", "left"];
    if (b.x + b.width <= a.x) return ["left", "right"];
    if (a.y + a.height <= b.y) return ["bottom", "top"];
    if (b.y + b.height <= a.y) return ["top", "bottom"];
    return ["right", "right"];
  }

  function sidePoint(box, side, along) {
    switch (side) {
      case "left":
        return { x: box.x, y: along };
      case "right":
        return { x: box.x + box.width, y: along };
      case "top":
        return { x: along, y: box.y };
      default:
        return { x: along, y: box.y + box.height };
    }
  }

  function bezierMid(p0, c0, c1, p1) {
    return { x: (p0.x + 3 * c0.x + 3 * c1.x + p1.x) / 8, y: (p0.y + 3 * c0.y + 3 * c1.y + p1.y) / 8 };
  }

  function pt(p) {
    return round(p.x) + "," + round(p.y);
  }

  // curvePath draws a smooth path from a, leaving along normal na, through
  // the waypoints, to b, arriving along normal nb. Between waypoints the
  // curve keeps to the main axis ("h" or "v").
  function curvePath(a, na, b, nb, waypoints, axis) {
    var points = [a].concat(waypoints || [], [b]);
    var d = "M" + pt(a);
    var mid = null;
    // The label sits on the middle segment of a layered path, and on the
    // first segment of a lane-crossing path, which lies between lanes.
    var middle = axis === "h" ? 0 : Math.floor((points.length - 2) / 2);
    // An end's control point reaches half the distance along its normal, so
    // the curve stays between its ends; an end facing away gets a fixed
    // reach so the curve can turn.
    function reach(from, to, n) {
      var along = (to.x - from.x) * n[0] + (to.y - from.y) * n[1];
      return along > 0 ? Math.max(10, along / 2) : 36;
    }
    for (var i = 0; i + 1 < points.length; i++) {
      var p = points[i];
      var q = points[i + 1];
      var c0;
      var c1;
      if (i === 0) {
        var r0 = reach(p, q, na);
        c0 = { x: p.x + na[0] * r0, y: p.y + na[1] * r0 };
      } else {
        c0 = axis === "h" ? { x: p.x + (q.x - p.x) / 2, y: p.y } : { x: p.x, y: p.y + (q.y - p.y) / 2 };
      }
      if (i + 2 === points.length) {
        var r1 = reach(q, p, nb);
        c1 = { x: q.x + nb[0] * r1, y: q.y + nb[1] * r1 };
      } else {
        c1 = axis === "h" ? { x: q.x - (q.x - p.x) / 2, y: q.y } : { x: q.x, y: q.y - (q.y - p.y) / 2 };
      }
      d += " C" + pt(c0) + " " + pt(c1) + " " + pt(q);
      if (i === middle) mid = bezierMid(p, c0, c1, q);
    }
    return { d: d, mid: mid };
  }

  // routeLinks draws each link between two drawn nodes. Edges sharing a
  // side of a node spread along it in the order of the nodes they lead to,
  // so fans of edges never start from one point. waypointsFor, when given,
  // returns the points an edge must pass through.
  function routeLinks(nodes, links, opts) {
    opts = opts || {};
    var byId = dict();
    nodes.forEach(function (n) {
      byId[n.id] = n;
    });
    var routes = [];
    var slots = dict();
    function slot(node, side, entry) {
      var key = node.id + "|" + side;
      (slots[key] = slots[key] || { node: node, side: side, entries: [] }).entries.push(entry);
    }
    links.forEach(function (link, index) {
      var a = byId[link.from];
      var b = byId[link.to];
      if (!a || !b) return;
      var sides = facingSides(a, b);
      var route = { link: link, index: index, a: a, b: b, sides: sides };
      routes.push(route);
      var ca = { x: a.x + a.width / 2, y: a.y + a.height / 2 };
      var cb = { x: b.x + b.width / 2, y: b.y + b.height / 2 };
      var vertical = function (side) {
        return side === "left" || side === "right";
      };
      slot(a, sides[0], { route: route, end: 0, order: a === b ? -1 : vertical(sides[0]) ? cb.y : cb.x });
      slot(b, sides[1], { route: route, end: 1, order: a === b ? 1 : vertical(sides[1]) ? ca.y : ca.x });
    });
    var gap = opts.portGap || 16;
    Object.keys(slots)
      .sort(byString)
      .forEach(function (key) {
        var s = slots[key];
        s.entries.sort(function (p, q) {
          return p.order - q.order || p.route.index - q.route.index || p.end - q.end;
        });
        var vertical = s.side === "left" || s.side === "right";
        var length = vertical ? s.node.height : s.node.width;
        var start = vertical ? s.node.y : s.node.x;
        var n = s.entries.length;
        var spacing = n > 1 ? Math.min(gap, (length - 12) / (n - 1)) : 0;
        var first = start + length / 2 - (spacing * (n - 1)) / 2;
        s.entries.forEach(function (entry, i) {
          var p = sidePoint(s.node, s.side, first + i * spacing);
          entry.route[entry.end === 0 ? "p1" : "p2"] = { x: round(p.x), y: round(p.y) };
        });
      });
    var edges = routes.map(function (r) {
      var waypoints = opts.waypointsFor ? opts.waypointsFor(r.link, r.p1, r.p2, r.a, r.b) : [];
      var path = curvePath(r.p1, NORMALS[r.sides[0]], r.p2, NORMALS[r.sides[1]], waypoints, opts.axis || "h");
      var label = r.link.label || "";
      var lines = label ? wrapText(label, opts.labelMax || 180, LABEL_SIZE, 3) : [];
      return {
        id: r.link.from + ">" + r.link.to + ">" + (r.link.via || ""),
        from: r.a.id,
        to: r.b.id,
        via: r.link.via || "",
        label: label,
        labelLines: lines,
        labelWidth: labelWidth(lines),
        change: r.link.change || "",
        faded: !!r.link.faded,
        refs: list(r.link.refs),
        x1: r.p1.x,
        y1: r.p1.y,
        x2: r.p2.x,
        y2: r.p2.y,
        d: path.d,
        labelX: round(path.mid.x),
        labelY: round(path.mid.y),
      };
    });
    stackParallelLabels(edges);
    return edges;
  }

  var LABEL_SIZE = 12;
  var LABEL_LINE = 14;

  function labelWidth(lines) {
    if (!lines.length) return 0;
    return round(Math.max.apply(null, lines.map(function (l) { return textWidth(l, LABEL_SIZE); })) + 14);
  }

  // stackParallelLabels keeps the labels of edges between the same two
  // nodes apart: stacked across the edges' direction around their shared
  // middle.
  function stackParallelLabels(edges) {
    var groups = dict();
    edges.forEach(function (e) {
      if (!e.labelLines.length || e.from === e.to) return;
      var key = e.from < e.to ? e.from + "\u0000" + e.to : e.to + "\u0000" + e.from;
      (groups[key] = groups[key] || []).push(e);
    });
    Object.keys(groups).forEach(function (key) {
      var group = groups[key];
      if (group.length < 2) return;
      group.sort(function (a, b) {
        return byString(a.id, b.id);
      });
      var mx = 0;
      var my = 0;
      group.forEach(function (e) {
        mx += e.labelX / group.length;
        my += e.labelY / group.length;
      });
      var e0 = group[0];
      var across = Math.abs(e0.x2 - e0.x1) >= Math.abs(e0.y2 - e0.y1) ? "y" : "x";
      var sizes = group.map(function (e) {
        return across === "y" ? e.labelLines.length * LABEL_LINE + 8 : e.labelWidth + 8;
      });
      var total = sizes.reduce(function (a, b) {
        return a + b;
      }, 0);
      var at = -total / 2;
      group.forEach(function (e, k) {
        var c = at + sizes[k] / 2;
        at += sizes[k];
        e.labelX = round(across === "x" ? mx + c : mx);
        e.labelY = round(across === "y" ? my + c : my);
      });
    });
  }

  function byTitleThenId(a, b) {
    return byString(String(a.title || "").toLowerCase(), String(b.title || "").toLowerCase()) || byString(a.id, b.id);
  }

  // linksAmong lists the links whose two ends both have drawn nodes, in a
  // fixed order whatever order the data gave them in.
  function linksAmong(data, ids) {
    var set = dict();
    ids.forEach(function (id) {
      set[id] = true;
    });
    return data.links
      .filter(function (l) {
        return set[l.from] && set[l.to];
      })
      .sort(function (a, b) {
        return byString(linkKey(a), linkKey(b));
      });
  }

  // stackColumns places columns of boxes left to right, each centred
  // vertically against the tallest.
  function stackColumns(columns, x0, y0, columnGap, rowGap) {
    var heights = columns.map(function (col) {
      return col.reduce(function (h, n, i) {
        return h + n.height + (i ? rowGap : 0);
      }, 0);
    });
    var tallest = Math.max.apply(null, heights.concat([0]));
    var x = x0;
    columns.forEach(function (col, c) {
      var width = Math.max.apply(null, col.map(function (n) { return n.width; }).concat([0]));
      var y = y0 + (tallest - heights[c]) / 2;
      col.forEach(function (n) {
        n.x = x + (width - n.width) / 2;
        n.y = y;
        y += n.height + rowGap;
      });
      if (col.length) x += width + columnGap;
    });
    return tallest;
  }

  // ----------------------------------------------------------- context view

  var CONTEXT = {
    margin: 40,
    columnGap: 230,
    rowGap: 22,
    nodeHeight: 62,
    minWidth: 210,
    systemWidth: 280,
    systemMinHeight: 170,
    personGap: 120,
    peopleGap: 48,
    splitAbove: 5,
  };

  var REMOTE_VIAS = ["http", "network"];

  // layoutContext arranges the system context by role: people above the
  // system, the outside services it reaches over the network on its right,
  // and the local programs it runs or that run it on its left.
  function layoutContext(data) {
    var c = CONTEXT;
    var nodes = data.nodes.filter(function (n) {
      return n.level === "context";
    });
    if (!nodes.length) {
      return emptyLayout("context", "No system context yet", "The context view fills in once content names the owner and the outside systems Evie talks to.");
    }
    nodes.sort(byTitleThenId);
    var people = [];
    var systems = [];
    var others = [];
    nodes.forEach(function (node) {
      (node.kind === "person" ? people : node.kind === "system" ? systems : others).push(node);
    });
    var systemIds = dict();
    systems.forEach(function (n) {
      systemIds[n.id] = true;
    });
    var links = linksAmong(
      data,
      nodes.map(function (n) {
        return n.id;
      })
    );
    var left = [];
    var right = [];
    others.forEach(function (node) {
      var via = "";
      links.forEach(function (l) {
        if (!via && ((l.from === node.id && systemIds[l.to]) || (l.to === node.id && systemIds[l.from]))) via = l.via;
      });
      (via && REMOTE_VIAS.indexOf(via) < 0 ? left : right).push(node);
    });
    if (!left.length && right.length > c.splitAbove) left = right.splice(0, Math.floor(right.length / 2));
    if (!right.length && left.length > c.splitAbove) right = left.splice(Math.ceil(left.length / 2));
    function box(node, min) {
      return drawnNode(node, Math.max(min, textWidth(node.title, 15) + 72), c.nodeHeight);
    }
    var leftBoxes = left.map(function (n) { return box(n, c.minWidth); });
    var rightBoxes = right.map(function (n) { return box(n, c.minWidth); });
    var peopleBoxes = people.map(function (n) { return box(n, c.minWidth); });
    var systemBoxes = systems.map(function (n) { return box(n, c.systemWidth); });
    function stackHeight(bs) {
      return bs.reduce(function (h, b, i) {
        return h + b.height + (i ? c.rowGap : 0);
      }, 0);
    }
    function widest(bs) {
      return Math.max.apply(null, bs.map(function (b) { return b.width; }).concat([0]));
    }
    var sides = Math.max(stackHeight(leftBoxes), stackHeight(rightBoxes));
    var fan = Math.max(leftBoxes.length, rightBoxes.length) + links.length / Math.max(systemBoxes.length, 1) / 3;
    systemBoxes.forEach(function (b) {
      var node = data.nodeById[b.id];
      b.summaryLines = wrapText(node.summary && node.summary.text, b.width - 36, 12.5, 6);
      b.height = round(Math.max(c.systemMinHeight, 74 + b.summaryLines.length * 17, (fan + 1) * 18));
    });
    var body = Math.max(sides, stackHeight(systemBoxes));
    var top = c.margin + (peopleBoxes.length ? c.nodeHeight + c.personGap : 0);
    var x = c.margin;
    function place(bs, colX, width) {
      var y = top + (body - stackHeight(bs)) / 2;
      bs.forEach(function (b) {
        b.x = colX + (width - b.width) / 2;
        b.y = y;
        y += b.height + c.rowGap;
      });
    }
    if (leftBoxes.length) {
      place(leftBoxes, x, widest(leftBoxes));
      x += widest(leftBoxes) + c.columnGap;
    }
    var middle = Math.max(widest(systemBoxes), c.systemWidth);
    place(systemBoxes, x, middle);
    var centreX = x + middle / 2;
    x += middle + c.columnGap;
    if (rightBoxes.length) place(rightBoxes, x, widest(rightBoxes));
    var rowWidth = peopleBoxes.reduce(function (w, b, i) {
      return w + b.width + (i ? c.peopleGap : 0);
    }, 0);
    var px = centreX - rowWidth / 2;
    peopleBoxes.forEach(function (b) {
      b.x = px;
      b.y = c.margin;
      px += b.width + c.peopleGap;
    });
    var drawn = peopleBoxes.concat(leftBoxes, systemBoxes, rightBoxes);
    var minX = Math.min.apply(null, drawn.map(function (b) { return b.x; }));
    drawn.forEach(function (b) {
      b.x += c.margin - Math.min(minX, c.margin);
    });
    var layout = {
      kind: "context",
      nodes: drawn,
      edges: routeLinks(drawn, links, { portGap: 44, labelMax: 190 }),
      lanes: [],
      empty: null,
    };
    return finishLayout(layout, c.margin);
  }

  // --------------------------------------------------------- containers view

  var CONTAINER_LANES = [
    { id: "people", title: "People", kinds: ["person"], inside: false },
    { id: "interfaces", title: "Interfaces and CLIs", kinds: ["user-interface", "cli"], inside: true },
    { id: "processes", title: "Processes", kinds: ["process"], inside: true },
    { id: "stores", title: "Stores", kinds: ["store"], inside: true },
    { id: "outside", title: "Outside services", kinds: ["external-system"], inside: false },
  ];

  var CONTAINERS = {
    margin: 32,
    laneGap: 96,
    lanePad: 20,
    laneHeader: 42,
    rowGap: 24,
    nodeHeight: 58,
    minWidth: 190,
    boundaryPad: 16,
    boundaryHeader: 34,
    passPad: 8,
  };

  function laneFor(node) {
    if (node.level === "context" && node.kind !== "person" && node.kind !== "external-system") return -1;
    for (var i = 0; i < CONTAINER_LANES.length; i++) {
      if (CONTAINER_LANES[i].kinds.indexOf(node.kind) >= 0) return i;
    }
    return node.level === "container" ? 2 : -1;
  }

  // orderByNeighbours reorders each column so a node sits near the nodes it
  // links to in the other columns: a few sweeps of the barycentre heuristic,
  // ties kept in their current order.
  function orderByNeighbours(columns, links, sweeps) {
    var neighbours = dict();
    links.forEach(function (l) {
      if (l.from === l.to) return;
      (neighbours[l.from] = neighbours[l.from] || []).push(l.to);
      (neighbours[l.to] = neighbours[l.to] || []).push(l.from);
    });
    function positions() {
      var pos = dict();
      columns.forEach(function (col) {
        col.forEach(function (n, i) {
          pos[n.id] = col.length > 1 ? i / (col.length - 1) : 0.5;
        });
      });
      return pos;
    }
    for (var s = 0; s < (sweeps || 4); s++) {
      var order = s % 2 === 0 ? columns.map(function (c, i) { return i; }) : columns.map(function (c, i) { return columns.length - 1 - i; });
      order.forEach(function (c) {
        var pos = positions();
        var col = columns[c];
        var keyed = col.map(function (n, i) {
          var others = (neighbours[n.id] || []).filter(function (id) {
            return pos[id] !== undefined && col.every(function (m) { return m.id !== id; });
          });
          var bary = others.length
            ? others.reduce(function (sum, id) { return sum + pos[id]; }, 0) / others.length
            : pos[n.id];
          return { node: n, key: bary, index: i };
        });
        keyed.sort(function (a, b) {
          return a.key - b.key || a.index - b.index;
        });
        columns[c] = keyed.map(function (k) {
          return k.node;
        });
      });
    }
    return columns;
  }

  // passThrough returns the waypoints an edge needs to cross the lanes
  // between its ends: one per lane, in the free space between that lane's
  // nodes nearest the straight line.
  function passThrough(lanes, p1, p2, pad) {
    var lo = Math.min(p1.x, p2.x);
    var hi = Math.max(p1.x, p2.x);
    var crossed = lanes.filter(function (lane) {
      return lane.x > lo && lane.x + lane.width < hi;
    });
    var forward = p2.x >= p1.x;
    if (!forward) crossed.reverse();
    var points = [];
    crossed.forEach(function (lane) {
      var x = lane.x + lane.width / 2;
      var t = (x - p1.x) / (p2.x - p1.x);
      var want = p1.y + (p2.y - p1.y) * t;
      var boxes = lane.boxes.slice().sort(function (a, b) {
        return a.y - b.y;
      });
      var gaps = [];
      var top = lane.y;
      boxes.forEach(function (b) {
        gaps.push([top, b.y - pad]);
        top = b.y + b.height + pad;
      });
      gaps.push([top, lane.y + lane.height + 400]);
      gaps[0][0] = lane.y - 400;
      var best = null;
      gaps.forEach(function (g) {
        if (g[1] < g[0]) return;
        var y = Math.min(Math.max(want, g[0]), g[1]);
        if (best === null || Math.abs(y - want) < Math.abs(best - want)) best = y;
      });
      // Enter and leave the lane at the same height, so the edge runs level
      // through the gap.
      var enter = forward ? lane.x : lane.x + lane.width;
      var leave = forward ? lane.x + lane.width : lane.x;
      points.push({ x: round(enter), y: round(best) }, { x: round(leave), y: round(best) });
    });
    return points;
  }

  // layoutContainers draws what runs and where data lives: lanes by kind
  // from the people who drive Evie to the outside services it calls, with
  // Evie's own lanes inside a system boundary.
  function layoutContainers(data) {
    var c = CONTAINERS;
    var columns = CONTAINER_LANES.map(function () {
      return [];
    });
    data.nodes.forEach(function (node) {
      if (node.level !== "container" && node.level !== "context") return;
      var lane = laneFor(node);
      if (lane < 0) return;
      columns[lane].push(drawnNode(node, Math.max(c.minWidth, textWidth(node.title, 14) + 64), c.nodeHeight));
    });
    var hasContainers = columns[1].length + columns[2].length + columns[3].length > 0;
    if (!hasContainers) {
      return emptyLayout("containers", "No containers yet", "The containers view fills in once content names Evie's processes, interfaces, command-line tools, and stores.");
    }
    columns.forEach(function (col) {
      col.sort(byTitleThenId);
    });
    var ids = [];
    columns.forEach(function (col) {
      col.forEach(function (n) {
        ids.push(n.id);
      });
    });
    var links = linksAmong(data, ids);
    orderByNeighbours(columns, links, 6);

    var kept = [];
    CONTAINER_LANES.forEach(function (def, i) {
      if (columns[i].length) kept.push({ def: def, nodes: columns[i] });
    });
    var bodyHeight = 0;
    kept.forEach(function (lane) {
      var h = lane.nodes.reduce(function (sum, n, i) {
        return sum + n.height + (i ? c.rowGap : 0);
      }, 0);
      bodyHeight = Math.max(bodyHeight, h);
    });
    var top = c.margin + c.boundaryHeader;
    var laneHeight = c.laneHeader + bodyHeight + c.lanePad;
    var x = c.margin;
    var lanes = [];
    var prevInside = false;
    kept.forEach(function (lane, i) {
      var inside = lane.def.inside;
      if (i > 0) x += inside !== prevInside ? c.laneGap + c.boundaryPad : c.laneGap;
      prevInside = inside;
      var width = Math.max.apply(null, lane.nodes.map(function (n) { return n.width; })) + 2 * c.lanePad;
      var h = lane.nodes.reduce(function (sum, n, k) {
        return sum + n.height + (k ? c.rowGap : 0);
      }, 0);
      var y = top + c.laneHeader + (bodyHeight - h) / 2;
      lane.nodes.forEach(function (n) {
        n.x = x + (width - n.width) / 2;
        n.y = y;
        y += n.height + c.rowGap;
      });
      lanes.push({
        id: lane.def.id,
        title: lane.def.title,
        inside: inside,
        x: round(x),
        y: round(top),
        width: round(width),
        height: round(laneHeight),
        nodes: lane.nodes.map(function (n) {
          return n.id;
        }),
        boxes: lane.nodes,
      });
      x += width;
    });
    var insideLanes = lanes.filter(function (l) {
      return l.inside;
    });
    var boundary = null;
    if (insideLanes.length) {
      var first = insideLanes[0];
      var last = insideLanes[insideLanes.length - 1];
      var system = data.nodes.filter(function (n) {
        return n.level === "context" && n.kind === "system";
      })[0];
      boundary = {
        title: system ? system.title : "System",
        ref: system ? system.id : null,
        x: round(first.x - c.boundaryPad),
        y: round(c.margin),
        width: round(last.x + last.width - first.x + 2 * c.boundaryPad),
        height: round(c.boundaryHeader + laneHeight + c.boundaryPad),
      };
    }
    var nodes = [];
    lanes.forEach(function (l) {
      nodes = nodes.concat(l.boxes);
    });
    var edges = routeLinks(nodes, links, {
      portGap: 16,
      waypointsFor: function (link, p1, p2) {
        return passThrough(lanes, p1, p2, c.passPad);
      },
    });
    lanes.forEach(function (l) {
      delete l.boxes;
    });
    var layout = finishLayout({ kind: "containers", nodes: nodes, edges: edges, lanes: lanes, boundary: boundary, empty: null }, c.margin);
    if (boundary) layout.height = Math.max(layout.height, round(boundary.y + boundary.height + c.margin));
    return layout;
  }

  // --------------------------------------------------------- layered engine
  //
  // A small Sugiyama-style layout for the views that read top to bottom
  // (group, inside a container, state and plain graph diagrams): break
  // cycles, rank by longest path, add a dummy node per layer a long edge
  // crosses, order each layer by barycentres to reduce crossings, place x by
  // least squares against neighbours, and route edges through their dummies.

  // isotonic returns the non-decreasing sequence closest (least squares) to
  // targets: pool adjacent violators.
  function isotonic(targets) {
    var blocks = [];
    targets.forEach(function (t) {
      blocks.push({ sum: t, n: 1 });
      while (blocks.length > 1) {
        var b = blocks[blocks.length - 1];
        var a = blocks[blocks.length - 2];
        if (a.sum / a.n <= b.sum / b.n) break;
        blocks.splice(blocks.length - 2, 2, { sum: a.sum + b.sum, n: a.n + b.n });
      }
    });
    var out = [];
    blocks.forEach(function (b) {
      for (var i = 0; i < b.n; i++) out.push(b.sum / b.n);
    });
    return out;
  }

  // placeRow puts a row's boxes, kept in order with at least gap between
  // them, as close as it can to the centres they want.
  // placeRow puts a row's boxes, kept in order with gap(a, b) between
  // neighbours, as close as it can to the centres they want.
  function placeRow(row, wanted, gap) {
    var offsets = [];
    var offset = 0;
    row.forEach(function (n, i) {
      if (i > 0) offset += row[i - 1].width + (row[i - 1].extraRight || 0) + gap(row[i - 1], n);
      offsets.push(offset);
    });
    var z = isotonic(
      row.map(function (n, i) {
        return wanted[i] - n.width / 2 - offsets[i];
      })
    );
    row.forEach(function (n, i) {
      n.x = z[i] + offsets[i];
    });
  }

  function crossingCount(upper, lower, segments) {
    var pu = dict();
    var pl = dict();
    upper.forEach(function (n, i) {
      pu[n.id] = i;
    });
    lower.forEach(function (n, i) {
      pl[n.id] = i;
    });
    var pairs = segments
      .filter(function (s) {
        return pu[s[0]] !== undefined && pl[s[1]] !== undefined;
      })
      .map(function (s) {
        return [pu[s[0]], pl[s[1]]];
      });
    var c = 0;
    for (var i = 0; i < pairs.length; i++) {
      for (var j = i + 1; j < pairs.length; j++) {
        if ((pairs[i][0] - pairs[j][0]) * (pairs[i][1] - pairs[j][1]) < 0) c++;
      }
    }
    return c;
  }

  // rankEdges ranks ids by longest path along edges ({from, to, index,
  // self}). A depth-first search first reverses the edges that lead back to
  // an id on its stack, so cycles rank too; each edge gets
  // reversed, top, and bottom. Self edges are left out of the ranking.
  function rankEdges(ids, edges) {
    var out = dict();
    var inn = dict();
    ids.forEach(function (id) {
      out[id] = [];
      inn[id] = [];
    });
    edges.forEach(function (e) {
      if (!e.self) out[e.from].push(e);
    });
    var state = dict();
    function visit(id) {
      state[id] = 1;
      out[id]
        .slice()
        .sort(function (a, b) {
          return byString(a.to, b.to) || a.index - b.index;
        })
        .forEach(function (e) {
          if (state[e.to] === 1) e.reversed = true;
          else if (!state[e.to]) visit(e.to);
        });
      state[id] = 2;
    }
    // Sources first, so a cycle is broken where the walk from a source
    // closes it, not at an arbitrary id.
    var indegree = dict();
    edges.forEach(function (e) {
      if (!e.self) indegree[e.to] = (indegree[e.to] || 0) + 1;
    });
    ids
      .filter(function (id) {
        return !indegree[id];
      })
      .concat(ids)
      .forEach(function (id) {
        if (!state[id]) visit(id);
      });
    ids.forEach(function (id) {
      out[id] = [];
      inn[id] = [];
    });
    edges.forEach(function (e) {
      if (e.self) return;
      e.top = e.reversed ? e.to : e.from;
      e.bottom = e.reversed ? e.from : e.to;
      out[e.top].push(e);
      inn[e.bottom].push(e);
    });
    var rank = dict();
    var pending = dict();
    ids.forEach(function (id) {
      pending[id] = inn[id].length;
    });
    var queue = ids.filter(function (id) {
      return pending[id] === 0;
    });
    while (queue.length) {
      queue.sort(byString);
      var id = queue.shift();
      if (rank[id] === undefined) rank[id] = 0;
      out[id].forEach(function (e) {
        rank[e.bottom] = Math.max(rank[e.bottom] || 0, rank[id] + 1);
        if (--pending[e.bottom] === 0) queue.push(e.bottom);
      });
    }
    ids.forEach(function (id) {
      if (rank[id] === undefined) rank[id] = 0;
    });
    return rank;
  }

  var LAYERED = { rankGap: 70, nodeGap: 28, dummyWidth: 6, dummyGap: 6, dummyNodeGap: 16, sweeps: 16, placeRounds: 10, portGap: 14 };

  // layered lays out boxes ({id, width, height, last}) joined by links
  // ({from, to}) from top to bottom. Boxes marked last share the bottom
  // layer. It sets each box's x and y and returns one route per link: the
  // points it passes through and the sides it leaves and arrives on.
  function layered(boxes, links, opts) {
    opts = opts || {};
    var rankGap = opts.rankGap || LAYERED.rankGap;
    var nodeGap = opts.nodeGap || LAYERED.nodeGap;
    var byId = dict();
    var ids = boxes
      .map(function (b) {
        byId[b.id] = b;
        return b.id;
      })
      .sort(byString);
    var edges = [];
    links.forEach(function (link, index) {
      if (!byId[link.from] || !byId[link.to]) return;
      edges.push({ link: link, index: index, from: link.from, to: link.to, self: link.from === link.to });
    });
    var rank = rankEdges(ids, edges);
    var dag = edges.filter(function (e) {
      return !e.self;
    });
    var maxRank = 0;
    ids.forEach(function (id) {
      if (!byId[id].last) maxRank = Math.max(maxRank, rank[id]);
    });
    var hasLast = ids.some(function (id) {
      return byId[id].last;
    });
    ids.forEach(function (id) {
      if (byId[id].last) rank[id] = maxRank + 1;
    });
    if (hasLast) maxRank += 1;
    // A layer with more boxes than fit across wraps onto extra rows: boxes
    // fed from above first, boxes feeding the layer below last, so edges
    // stay short.
    var perRow = opts.maxPerRow || 0;
    if (perRow) {
      var balance = dict();
      dag.forEach(function (e) {
        balance[e.top] = (balance[e.top] || 0) + 1;
        balance[e.bottom] = (balance[e.bottom] || 0) - 1;
      });
      var next = 0;
      var remap = dict();
      for (var lr = 0; lr <= maxRank; lr++) {
        var members = ids
          .filter(function (id) {
            return rank[id] === lr;
          })
          .sort(function (a, b) {
            return (balance[a] || 0) - (balance[b] || 0) || byString(byId[a].sortKey || a, byId[b].sortKey || b);
          });
        if (!members.length) continue;
        var subRows = Math.ceil(members.length / perRow);
        var size = Math.ceil(members.length / subRows);
        members.forEach(function (id, i) {
          remap[id] = next + Math.floor(i / size);
        });
        next += subRows;
      }
      ids.forEach(function (id) {
        rank[id] = remap[id];
      });
      maxRank = next - 1;
    }
    // Rows of boxes and dummies.
    var rows = [];
    for (var r = 0; r <= maxRank; r++) rows.push([]);
    ids.forEach(function (id) {
      rows[rank[id]].push(byId[id]);
    });
    var segments = [];
    dag.forEach(function (e) {
      var chain = [e.top];
      for (var k = rank[e.top] + 1; k < rank[e.bottom]; k++) {
        var dummy = { id: "~" + e.index + "~" + k, width: LAYERED.dummyWidth, height: 0, dummy: true, toward: e.bottom };
        byId[dummy.id] = dummy;
        rows[k].push(dummy);
        chain.push(dummy.id);
      }
      chain.push(e.bottom);
      e.chain = chain;
      for (var i = 0; i + 1 < chain.length; i++) segments.push([chain[i], chain[i + 1]]);
    });
    var up = dict();
    var down = dict();
    segments.forEach(function (s) {
      (down[s[0]] = down[s[0]] || []).push(s[1]);
      (up[s[1]] = up[s[1]] || []).push(s[0]);
    });
    // Start each row with its boxes in name order and its dummies spread
    // evenly between them, so edges passing a wrapped layer go between its
    // boxes rather than around one end.
    var relative = dict();
    rows.forEach(function (row) {
      var real = row
        .filter(function (n) {
          return !n.dummy;
        })
        .sort(function (a, b) {
          return byString(a.sortKey || a.id, b.sortKey || b.id);
        });
      real.forEach(function (n, i) {
        relative[n.id] = (i + 0.5) / real.length;
      });
    });
    rows = rows.map(function (row) {
      var real = row.filter(function (n) {
        return !n.dummy;
      });
      var dummies = row.filter(function (n) {
        return n.dummy;
      });
      var placed = real
        .map(function (n) {
          return { n: n, at: relative[n.id], tie: 0 };
        })
        .concat(
          dummies.map(function (n) {
            return { n: n, at: relative[n.toward], tie: 1 };
          })
        );
      placed.sort(function (a, b) {
        return a.at - b.at || a.tie - b.tie || byString(a.n.id, b.n.id);
      });
      return placed.map(function (p) {
        return p.n;
      });
    });
    function total(rs) {
      var c = 0;
      for (var i = 0; i + 1 < rs.length; i++) c += crossingCount(rs[i], rs[i + 1], segments);
      return c;
    }
    function reorder(rs, i, neighbours, ref) {
      var pos = dict();
      ref.forEach(function (n, k) {
        pos[n.id] = k;
      });
      var keyed = rs[i].map(function (n, k) {
        var ns = (neighbours[n.id] || []).filter(function (m) {
          return pos[m] !== undefined;
        });
        var key = ns.length
          ? ns.reduce(function (s, m) {
              return s + pos[m];
            }, 0) / ns.length
          : (k * ref.length) / Math.max(rs[i].length, 1);
        return { n: n, key: key, k: k };
      });
      keyed.sort(function (a, b) {
        return a.key - b.key || a.k - b.k;
      });
      rs[i] = keyed.map(function (x) {
        return x.n;
      });
    }
    var best = rows.map(function (row) {
      return row.slice();
    });
    var bestCount = total(rows);
    for (var s = 0; s < LAYERED.sweeps && bestCount > 0; s++) {
      if (s % 2 === 0) {
        for (var i = 1; i < rows.length; i++) reorder(rows, i, up, rows[i - 1]);
      } else {
        for (var j = rows.length - 2; j >= 0; j--) reorder(rows, j, down, rows[j + 1]);
      }
      var count = total(rows);
      if (count < bestCount) {
        bestCount = count;
        best = rows.map(function (row) {
          return row.slice();
        });
      }
    }
    rows = best;
    // Place y by row and x by least squares against neighbours.
    var y = 0;
    var rowTop = [];
    var rowHeight = [];
    rows.forEach(function (row, i) {
      var h = Math.max.apply(null, row.map(function (n) { return n.height; }).concat([0]));
      rowTop[i] = y;
      rowHeight[i] = h;
      row.forEach(function (n) {
        n.y = y;
      });
      y += h + rankGap;
    });
    function gap(a, b) {
      if (a.dummy && b.dummy) return LAYERED.dummyGap;
      if (a.dummy || b.dummy) return Math.min(nodeGap, LAYERED.dummyNodeGap);
      return nodeGap;
    }
    rows.forEach(function (row) {
      var x = 0;
      row.forEach(function (n, i) {
        if (i > 0) x += gap(row[i - 1], n);
        n.x = x;
        x += n.width + (n.extraRight || 0);
      });
    });
    var widest = Math.max.apply(null, rows.map(function (row) {
      return row.length ? row[row.length - 1].x + row[row.length - 1].width : 0;
    }).concat([0]));
    rows.forEach(function (row) {
      var w = row.length ? row[row.length - 1].x + row[row.length - 1].width : 0;
      row.forEach(function (n) {
        n.x += (widest - w) / 2;
      });
    });
    function centre(id) {
      return byId[id].x + byId[id].width / 2;
    }
    function settle(row, neighbourSets) {
      var wanted = row.map(function (n) {
        var ns = [];
        neighbourSets.forEach(function (set) {
          ns = ns.concat(set[n.id] || []);
        });
        if (!ns.length) return n.x + n.width / 2;
        return (
          ns.reduce(function (sum, m) {
            return sum + centre(m);
          }, 0) / ns.length
        );
      });
      placeRow(row, wanted, gap);
    }
    for (var round_ = 0; round_ < LAYERED.placeRounds; round_++) {
      var k;
      if (round_ % 2 === 0) {
        for (k = 1; k < rows.length; k++) settle(rows[k], [up]);
      } else {
        for (k = rows.length - 2; k >= 0; k--) settle(rows[k], [down]);
      }
    }
    for (k = 0; k < rows.length; k++) settle(rows[k], [up, down]);
    var minX = Infinity;
    rows.forEach(function (row) {
      row.forEach(function (n) {
        minX = Math.min(minX, n.x);
      });
    });
    if (minX === Infinity) minX = 0;
    rows.forEach(function (row) {
      row.forEach(function (n) {
        n.x = round(n.x - minX);
        n.y = round(n.y);
      });
    });
    // Ports: the edges leaving a box's bottom spread along it in the order
    // of where they go next; likewise those arriving at its top.
    var bottomPorts = dict();
    var topPorts = dict();
    dag.forEach(function (e) {
      var first = e.chain[0];
      var second = e.chain[1];
      var last = e.chain[e.chain.length - 1];
      var before = e.chain[e.chain.length - 2];
      (bottomPorts[first] = bottomPorts[first] || []).push({ e: e, toward: centre(second) });
      (topPorts[last] = topPorts[last] || []).push({ e: e, toward: centre(before) });
    });
    function spread(ports, key) {
      Object.keys(ports).forEach(function (id) {
        var box = byId[id];
        var list_ = ports[id].sort(function (a, b) {
          return a.toward - b.toward || a.e.index - b.e.index;
        });
        var n = list_.length;
        var spacing = n > 1 ? Math.min(LAYERED.portGap, (box.width - 16) / (n - 1)) : 0;
        var first = box.x + box.width / 2 - (spacing * (n - 1)) / 2;
        list_.forEach(function (p, i) {
          p.e[key] = round(first + i * spacing);
        });
      });
    }
    spread(bottomPorts, "outX");
    spread(topPorts, "inX");
    var routes = edges.map(function (e) {
      if (e.self) {
        var b = byId[e.from];
        return {
          link: e.link,
          self: true,
          points: [
            { x: round(b.x + b.width), y: round(b.y + b.height * 0.3) },
            { x: round(b.x + b.width), y: round(b.y + b.height * 0.7) },
          ],
        };
      }
      var top = byId[e.top];
      var bottom = byId[e.bottom];
      var pts = [{ x: e.outX, y: round(top.y + top.height) }];
      var topRow = rank[e.top];
      if (top.height < rowHeight[topRow]) pts.push({ x: e.outX, y: round(rowTop[topRow] + rowHeight[topRow]) });
      for (var c = 1; c + 1 < e.chain.length; c++) {
        var d = byId[e.chain[c]];
        var dx = round(d.x + d.width / 2);
        var rr = rank[e.top] + c;
        pts.push({ x: dx, y: round(rowTop[rr]) }, { x: dx, y: round(rowTop[rr] + rowHeight[rr]) });
      }
      pts.push({ x: e.inX, y: round(bottom.y) });
      if (e.reversed) pts.reverse();
      return { link: e.link, reversed: !!e.reversed, points: pts };
    });
    return { routes: routes, height: y - rankGap, ranks: rank };
  }

  // layeredEdge turns a layered route into a drawn edge.
  function layeredEdge(route, from, to, extra) {
    var pts = route.points;
    var a = pts[0];
    var b = pts[pts.length - 1];
    var path;
    if (route.self) {
      var reach = 46;
      var c0 = { x: a.x + reach, y: a.y - 14 };
      var c1 = { x: b.x + reach, y: b.y + 14 };
      path = { d: "M" + pt(a) + " C" + pt(c0) + " " + pt(c1) + " " + pt(b), mid: bezierMid(a, c0, c1, b) };
    } else {
      var na = route.reversed ? NORMALS.top : NORMALS.bottom;
      var nb = route.reversed ? NORMALS.bottom : NORMALS.top;
      path = curvePath(a, na, b, nb, pts.slice(1, -1), "v");
    }
    var link = route.link;
    var label = link.label || "";
    var lines = label ? wrapText(label, 150, LABEL_SIZE, 3) : [];
    var out = {
      id: link.from + ">" + link.to + ">" + (link.via || ""),
      from: from,
      to: to,
      via: link.via || "",
      label: label,
      labelLines: lines,
      labelWidth: labelWidth(lines),
      change: link.change || "",
      faded: !!link.faded,
      refs: list(link.refs),
      x1: a.x,
      y1: a.y,
      x2: b.x,
      y2: b.y,
      d: path.d,
      labelX: round(path.mid.x + (route.self ? 14 : 0)),
      labelY: round(path.mid.y),
    };
    copy(extra, out);
    if (route.self) out.extent = [{ x: a.x + 60, y: b.y }];
    return out;
  }

  // ------------------------------------------------------------- group view

  var GROUP_VIEW = { margin: 36, nodeHeight: 54, chipHeight: 38, minWidth: 140, rankGap: 64, nodeGap: 26, rowBudget: 1400, chipGap: 12, chipRowGap: 12 };

  function packageBox(node, data, boundary) {
    var path = packagePath(node.id);
    var subtitle = boundary ? (node.group ? groupLabel(node.group) : kindLabel(node.kind)) : path || kindLabel(node.kind);
    // Room for the icon (packages and containers) or the chip's padding.
    var inset = boundary ? 34 : 58;
    var width = Math.max(GROUP_VIEW.minWidth, textWidth(node.title, 14) + inset, textWidth(subtitle, 11) + inset);
    var box = drawnNode(node, width, boundary ? GROUP_VIEW.chipHeight : GROUP_VIEW.nodeHeight, {
      subtitle: subtitle,
      boundary: !!boundary,
      faded: !!boundary,
      command: !!(path && data.packageByPath[path] && data.packageByPath[path].command),
      testSupport: !!(path && data.packageByPath[path] && data.packageByPath[path].testSupport),
    });
    box.last = !!boundary;
    box.sortKey = String(node.title || node.id).toLowerCase() + "\u0000" + node.id;
    return box;
  }

  // layoutMembers lays out a set of nodes by import depth, with what they
  // depend on outside the set as faded chips in rows below, each chip under
  // the members that use it. Edges to chips are drawn on demand: when a
  // member is hovered or selected.
  function layoutMembers(data, kind, memberIds, title) {
    var g = GROUP_VIEW;
    var member = dict();
    memberIds.forEach(function (id) {
      member[id] = true;
    });
    var boxes = [];
    memberIds
      .slice()
      .sort(byString)
      .forEach(function (id) {
        if (data.nodeById[id]) boxes.push(packageBox(data.nodeById[id], data, false));
      });
    var outside = dict();
    var links = [];
    var chipLinks = [];
    data.links.forEach(function (l) {
      if (!member[l.from] || l.from === l.to) return;
      if (member[l.to]) {
        links.push(l);
      } else if (data.nodeById[l.to]) {
        outside[l.to] = true;
        chipLinks.push(l);
      }
    });
    links.sort(function (a, b) {
      return byString(linkKey(a), linkKey(b));
    });
    chipLinks.sort(function (a, b) {
      return byString(linkKey(a), linkKey(b));
    });
    function perRow(bs) {
      var average = bs.reduce(function (sum, b) {
        return sum + b.width;
      }, 0) / Math.max(bs.length, 1);
      return Math.max(3, Math.floor(g.rowBudget / (average + g.nodeGap)));
    }
    var result = layered(boxes, links, { rankGap: g.rankGap, nodeGap: g.nodeGap, maxPerRow: perRow(boxes) });
    var byId = dict();
    boxes.forEach(function (b) {
      byId[b.id] = b;
    });
    // Chips in rows below, each under the mean of the members that use it.
    var chips = Object.keys(outside)
      .sort(byString)
      .map(function (id) {
        var chip = packageBox(data.nodeById[id], data, true);
        var users = chipLinks.filter(function (l) {
          return l.to === id;
        });
        chip.want = users.reduce(function (sum, l) {
          return sum + byId[l.from].x + byId[l.from].width / 2;
        }, 0) / users.length;
        return chip;
      })
      .sort(function (a, b) {
        return a.want - b.want || byString(a.sortKey, b.sortKey);
      });
    var chipTop = result.height + (boxes.length ? g.rankGap : 0);
    var cap = perRow(chips);
    var rowsOfChips = Math.ceil(chips.length / cap);
    var size = Math.ceil(chips.length / Math.max(rowsOfChips, 1));
    for (var r = 0; r < rowsOfChips; r++) {
      var row = chips.slice(r * size, (r + 1) * size);
      row.forEach(function (c) {
        c.y = chipTop + r * (g.chipHeight + g.chipRowGap);
      });
      placeRow(
        row,
        row.map(function (c) {
          return c.want;
        }),
        function () {
          return g.chipGap;
        }
      );
    }
    var all = boxes.concat(chips);
    var minX = Math.min.apply(null, all.map(function (b) { return b.x; }).concat([0]));
    all.forEach(function (b) {
      b.x = round(b.x - minX + g.margin);
      b.y = round(b.y + g.margin);
      delete b.last;
      delete b.sortKey;
      delete b.want;
    });
    var edges = result.routes.map(function (route) {
      route.points = route.points.map(function (p) {
        return { x: round(p.x - minX + g.margin), y: round(p.y + g.margin) };
      });
      return layeredEdge(route, route.link.from, route.link.to);
    });
    chips.forEach(function (c) {
      byId[c.id] = c;
    });
    chipLinks.forEach(function (l) {
      var a = byId[l.from];
      var b = byId[l.to];
      var p1 = { x: round(a.x + a.width / 2), y: round(a.y + a.height) };
      var p2 = { x: round(b.x + b.width / 2), y: round(b.y) };
      var faded = copy(l);
      faded.faded = true;
      var edge = layeredEdge({ link: faded, points: [p1, p2] }, l.from, l.to);
      edge.onDemand = true;
      edges.push(edge);
    });
    return finishLayout({ kind: kind, title: title, nodes: all, edges: edges, lanes: [], empty: null }, g.margin);
  }

  // layoutGroup draws one component group: its packages layered by import
  // depth, importers above what they import, and the packages they depend on
  // in other groups as faded chips below.
  function layoutGroup(data, groupId) {
    var label = groupLabel(groupId);
    var ids = data.nodes
      .filter(function (n) {
        return n.level === "component" && (n.group === groupId || n.alsoIn.indexOf(groupId) >= 0);
      })
      .map(function (n) {
        return n.id;
      });
    if (!ids.length) {
      return emptyLayout("group", "No packages in " + label + " yet", "Packages appear here once a content file gives them this group.");
    }
    return layoutMembers(data, "group", ids, label);
  }

  // layoutInside draws what a container zooms into: the nodes it contains,
  // laid out like a group.
  function layoutInside(data, containerId) {
    var container = data.nodeById[containerId];
    var ids = container
      ? container.contains.filter(function (id) {
          return !!data.nodeById[id];
        })
      : [];
    if (!ids.length) {
      return emptyLayout(
        "inside",
        container ? "Nothing inside " + container.title + " yet" : "No such container",
        "A container shows the nodes its contains list names; this one names none yet."
      );
    }
    return layoutMembers(data, "inside", ids, container.title);
  }

  // ------------------------------------------------------------- focus view

  var FOCUS = { margin: 36, columnGap: 230, chipHeight: 44, chipGap: 12, minChip: 170, focusWidth: 300, header: 34, portGap: 14 };

  // layoutFocus draws one node with who calls it on the left and what it
  // depends on on the right, from the import graph and the content edges.
  function layoutFocus(data, id) {
    var f = FOCUS;
    var node = data.nodeById[id];
    if (!node) return emptyLayout("focus", "No such node", "The address names a node the data does not have.");
    var seen = { callers: dict(), dependencies: dict() };
    var links = [];
    data.links.forEach(function (l) {
      if (l.from === l.to) return;
      if (l.to === id && data.nodeById[l.from]) {
        seen.callers[l.from] = true;
        links.push(l);
      } else if (l.from === id && data.nodeById[l.to]) {
        seen.dependencies[l.to] = true;
        links.push(l);
      }
    });
    function column(side) {
      return Object.keys(seen[side])
        .map(function (other) {
          var n = data.nodeById[other];
          var subtitle = n.group ? groupLabel(n.group) : kindLabel(n.kind);
          return drawnNode(n, Math.max(f.minChip, textWidth(n.title, 13.5) + 62, textWidth(subtitle, 11) + 62), f.chipHeight, {
            side: side,
            subtitle: subtitle,
          });
        })
        .sort(function (a, b) {
          return groupIndex(a.group) - groupIndex(b.group) || byTitleThenId(a, b);
        });
    }
    var callers = column("callers");
    var deps = column("dependencies");
    var summary = node.summary && node.summary.text ? node.summary.text : "";
    var summaryLines = wrapText(summary, f.focusWidth - 36, 12.5, 6);
    var ports = Math.max(callers.length, deps.length);
    var focusHeight = Math.max(96 + summaryLines.length * 17, (ports + 1) * f.portGap);
    var path = packagePath(id);
    var focus = drawnNode(node, f.focusWidth, focusHeight, {
      focus: true,
      side: "focus",
      subtitle: path || kindLabel(node.kind),
      summaryLines: summaryLines,
    });
    var top = f.margin + f.header;
    var colHeight = function (col) {
      return col.reduce(function (h, n, i) {
        return h + n.height + (i ? f.chipGap : 0);
      }, 0);
    };
    var tallest = Math.max(colHeight(callers), colHeight(deps), focusHeight);
    var leftWidth = Math.max.apply(null, callers.map(function (n) { return n.width; }).concat([f.minChip]));
    var rightWidth = Math.max.apply(null, deps.map(function (n) { return n.width; }).concat([f.minChip]));
    var x = f.margin;
    function stack(col, colX, width) {
      var y = top + (tallest - colHeight(col)) / 2;
      col.forEach(function (n) {
        n.x = colX + (width - n.width) / 2;
        n.y = y;
        y += n.height + f.chipGap;
      });
    }
    stack(callers, x, leftWidth);
    focus.x = x + leftWidth + f.columnGap;
    focus.y = top + (tallest - focusHeight) / 2;
    var rightX = focus.x + focus.width + f.columnGap;
    stack(deps, rightX, rightWidth);
    var nodes = callers.concat([focus], deps);
    links.sort(function (a, b) {
      return byString(linkKey(a), linkKey(b));
    });
    var columns = [
      { id: "callers", title: "Who calls it", count: callers.length, x: round(x), y: f.margin, width: round(leftWidth) },
      { id: "dependencies", title: "Depends on", count: deps.length, x: round(rightX), y: f.margin, width: round(rightWidth) },
    ];
    var layout = {
      kind: "focus",
      title: node.title,
      nodes: nodes,
      edges: routeLinks(nodes, links, { portGap: f.portGap, labelMax: 180 }),
      columns: columns,
      lanes: [],
      empty: null,
    };
    finishLayout(layout, f.margin);
    layout.width = Math.max(layout.width, round(rightX + rightWidth + f.margin));
    layout.height = Math.max(layout.height, round(top + tallest + f.margin));
    return layout;
  }

  // ---------------------------------------------------------- sequence view

  var SEQUENCE = {
    margin: 32,
    headHeight: 54,
    minHead: 124,
    headGap: 36,
    labelMax: 200,
    labelSize: 12,
    lineHeight: 15,
    rowPad: 30,
    selfReach: 44,
    selfDrop: 26,
    noteMax: 260,
    notePad: 36,
    firstRow: 34,
    tail: 34,
    farSpan: 260,
    labelInset: 30,
  };

  // flowIndex lists every flow for the flows view, by title.
  function flowIndex(data) {
    return data.flows
      .map(function (f) {
        return {
          id: f.id,
          title: f.title || f.id,
          summary: f.summary || null,
          trigger: f.trigger || null,
          participants: list(f.participants).length,
          steps: list(f.steps).length,
          change: f.change || "",
        };
      })
      .sort(byTitleThenId);
  }

  // layoutSequence draws a flow, or a sequence diagram, with one lifeline per
  // participant and its steps numbered top to bottom. Gaps between lifelines
  // grow until every step's label fits between the lifelines it joins.
  function layoutSequence(data, flow) {
    var q = SEQUENCE;
    var ids = list(flow && flow.participants);
    if (!ids.length) return emptyLayout("sequence", "No participants yet", "A flow needs participants and steps to draw.");
    var index = dict();
    var participants = ids.map(function (id, i) {
      index[id] = i;
      var node = data.nodeById[id];
      var title = node ? node.title || id : id;
      var subtitle = node ? (node.group ? groupLabel(node.group) : kindLabel(node.kind)) : "";
      return {
        id: id,
        ref: node ? id : null,
        title: title,
        subtitle: subtitle,
        kind: node ? node.kind || "" : "",
        group: node ? node.group || "" : "",
        change: node ? node.change || "" : "",
        x: 0,
        y: q.margin,
        // Even whole widths keep each lifeline exactly in its head's middle.
        width: 2 * Math.ceil(Math.max(q.minHead, textWidth(title, 13.5) + 58, textWidth(subtitle, 11) + 58) / 2),
        height: q.headHeight,
      };
    });
    var steps = list(flow.steps)
      .map(function (step, i) {
        return { step: step, number: i + 1 };
      })
      .filter(function (s) {
        return index[s.step.from] !== undefined && index[s.step.to] !== undefined;
      });
    // Gaps between neighbouring lifelines: room for the heads, then for each
    // label, narrowest spans first.
    var gaps = [];
    for (var i = 0; i + 1 < participants.length; i++) {
      gaps.push((participants[i].width + participants[i + 1].width) / 2 + q.headGap);
    }
    var tailRoom = 0;
    var laid = steps.map(function (s) {
      var step = s.step;
      var a = index[step.from];
      var b = index[step.to];
      var kind = step.kind || "call";
      var self = kind !== "note" && a === b;
      var width = kind === "note" ? q.noteMax : q.labelMax;
      var lines = wrapText(step.label || "", width, q.labelSize, 4);
      var labelWidth = Math.max.apply(null, lines.map(function (l) { return textWidth(l, q.labelSize); }).concat([0]));
      return { s: s, a: a, b: b, kind: kind, self: self, lines: lines, labelWidth: labelWidth };
    });
    laid
      .filter(function (l) {
        return l.kind !== "note" && !l.self;
      })
      .sort(function (p, r) {
        return Math.abs(p.a - p.b) - Math.abs(r.a - r.b) || p.s.number - r.s.number;
      })
      .forEach(function (l) {
        var lo = Math.min(l.a, l.b);
        var hi = Math.max(l.a, l.b);
        var have = 0;
        for (var g = lo; g < hi; g++) have += gaps[g];
        var need = l.labelWidth + 40;
        if (need > have) {
          for (g = lo; g < hi; g++) gaps[g] += (need - have) / (hi - lo);
        }
      });
    laid.forEach(function (l) {
      if (!l.self) return;
      var need = q.selfReach + l.labelWidth + 30;
      if (l.a < gaps.length) gaps[l.a] = Math.max(gaps[l.a], need + participants[l.a + 1].width / 2 - 10);
      else tailRoom = Math.max(tailRoom, need);
    });
    var x = q.margin + participants[0].width / 2;
    participants.forEach(function (p, k) {
      if (k > 0) x += gaps[k - 1];
      p.lifeline = round(x);
      p.x = round(x - p.width / 2);
    });
    var top = q.margin + q.headHeight;
    var y = top + q.firstRow;
    var out = laid.map(function (l) {
      var step = l.s.step;
      var from = participants[l.a];
      var to = participants[l.b];
      var labelHeight = l.lines.length * q.lineHeight;
      var dir = l.b >= l.a ? 1 : -1;
      var drawn = {
        number: l.s.number,
        from: step.from,
        to: step.to,
        kind: l.kind,
        via: step.via || "",
        label: step.label || "",
        lines: l.lines,
        labelWidth: round(l.labelWidth),
        refs: list(step.refs),
        change: step.change || "",
        self: l.self,
      };
      if (l.kind === "note") {
        var lo = Math.min(from.lifeline, to.lifeline) - q.notePad;
        var hi = Math.max(from.lifeline, to.lifeline) + q.notePad;
        var noteWidth = Math.max(hi - lo, l.labelWidth + 28);
        var mid = (lo + hi) / 2;
        var noteHeight = labelHeight + 16;
        drawn.box = { x: round(mid - noteWidth / 2), y: round(y - 6), width: round(noteWidth), height: round(noteHeight) };
        drawn.y = round(y - 6 + noteHeight / 2);
        drawn.labelX = round(mid);
        drawn.labelY = round(y + 8);
        y += noteHeight + q.rowPad - 6;
        return drawn;
      }
      y += labelHeight;
      drawn.y = round(y);
      drawn.x1 = from.lifeline;
      drawn.x2 = to.lifeline;
      if (l.self) {
        var reach = drawn.x1 + q.selfReach;
        drawn.y2 = round(y + q.selfDrop);
        drawn.d =
          "M" + drawn.x1 + "," + drawn.y +
          " C" + reach + "," + drawn.y + " " + reach + "," + drawn.y2 + " " + drawn.x2 + "," + drawn.y2;
        drawn.labelX = round(reach + 10);
        drawn.labelY = round(y - labelHeight + q.lineHeight - 3);
        drawn.anchor = "start";
        y += q.selfDrop;
      } else {
        drawn.d =
          "M" + drawn.x1 + "," + drawn.y +
          " C" + round(drawn.x1 + dir * 10) + "," + drawn.y + " " + round(drawn.x2 - dir * 10) + "," + drawn.y + " " + drawn.x2 + "," + drawn.y;
        // A message across many lifelines is labelled near its sender, so
        // the label is in view where the message starts.
        var span = Math.abs(drawn.x2 - drawn.x1);
        drawn.labelX = span > l.labelWidth + q.farSpan
          ? round(drawn.x1 + dir * (l.labelWidth / 2 + q.labelInset))
          : round((drawn.x1 + drawn.x2) / 2);
        drawn.labelY = round(y - labelHeight - 1 + q.lineHeight - 4);
        drawn.anchor = "middle";
      }
      y += q.rowPad;
      return drawn;
    });
    var bottom = y + q.tail - q.rowPad;
    participants.forEach(function (p) {
      p.lifelineTop = round(top);
      p.lifelineBottom = round(bottom);
    });
    var width = participants[participants.length - 1].x + participants[participants.length - 1].width + q.margin + tailRoom;
    out.forEach(function (s) {
      if (s.box) width = Math.max(width, s.box.x + s.box.width + q.margin);
    });
    return {
      kind: "sequence",
      title: flow.title || "",
      nodes: participants,
      participants: participants,
      steps: out,
      edges: [],
      lanes: [],
      width: round(width),
      height: round(bottom + q.margin),
      empty: null,
    };
  }

  // ----------------------------------------------------------- diagram views

  // findDiagram finds a diagram by id and the node that carries it.
  function findDiagram(data, id) {
    var ids = Object.keys(data.nodeById).sort(byString);
    for (var i = 0; i < ids.length; i++) {
      var node = data.nodeById[ids[i]];
      var diagrams = list(node.diagrams);
      for (var j = 0; j < diagrams.length; j++) {
        if (diagrams[j] && diagrams[j].id === id) return { node: node, diagram: diagrams[j] };
      }
    }
    return null;
  }

  var DIAGRAM = { margin: 32, nodeHeight: 46, minWidth: 110, maxLabel: 210, lineHeight: 16, rankGap: 74, nodeGap: 40, loopRoom: 60 };

  // diagramBox makes a node of a graph or state diagram, its label wrapped
  // onto lines no wider than the diagram's label width.
  function diagramBox(id, title, ref, data) {
    var target = ref ? data.nodeById[ref] : null;
    var lines = wrapText(title, DIAGRAM.maxLabel, 13, 5);
    if (!lines.length) lines = [""];
    var widest = Math.max.apply(null, lines.map(function (l) { return textWidth(l, 13); }));
    var subtitle = target && lines.length === 1 ? target.title : "";
    return {
      id: id,
      ref: target ? ref : null,
      title: title,
      lines: lines,
      subtitle: subtitle,
      kind: target ? target.kind || "" : "",
      group: target ? target.group || "" : "",
      change: target ? target.change || "" : "",
      shape: "part",
      x: 0,
      y: 0,
      width: round(Math.max(DIAGRAM.minWidth, widest + 34, textWidth(subtitle, 11) + 34)),
      height: round(Math.max(DIAGRAM.nodeHeight, lines.length * DIAGRAM.lineHeight + 22)),
    };
  }

  // layoutLayeredDiagram lays out a plain graph or a state diagram top to
  // bottom with the layered engine.
  function layoutLayeredDiagram(kind, boxes, links, title) {
    var d = DIAGRAM;
    var selfLoops = dict();
    links.forEach(function (l) {
      if (l.from === l.to) selfLoops[l.from] = Math.max(selfLoops[l.from] || 0, textWidth(l.label, 11) + d.loopRoom);
    });
    boxes.forEach(function (b) {
      if (selfLoops[b.id]) b.extraRight = round(selfLoops[b.id]);
    });
    var result = layered(boxes, links, { rankGap: d.rankGap, nodeGap: d.nodeGap });
    boxes.forEach(function (b) {
      b.x = round(b.x + d.margin);
      b.y = round(b.y + d.margin);
      delete b.extraRight;
    });
    var edges = result.routes.map(function (route) {
      route.points = route.points.map(function (p) {
        return { x: round(p.x + d.margin), y: round(p.y + d.margin) };
      });
      return layeredEdge(route, route.link.from, route.link.to);
    });
    return finishLayout({ kind: kind, title: title, nodes: boxes, edges: edges, lanes: [], empty: null }, d.margin);
  }

  function graphLinks(diagram, known) {
    return list(diagram.edges)
      .filter(function (e) {
        return known[e.from] && known[e.to];
      })
      .map(function (e) {
        return { from: e.from, to: e.to, label: e.label || "", refs: list(e.refs), via: "" };
      });
  }

  function layoutGraphDiagram(data, diagram) {
    var known = dict();
    var boxes = list(diagram.nodes).map(function (n) {
      known[n.id] = true;
      return diagramBox(n.id, n.label || n.id, n.node, data);
    });
    if (!boxes.length) return emptyLayout("graph", "Nothing to draw yet", "This diagram lists no nodes.");
    var links = graphLinks(diagram, known);
    if (list(diagram.lanes).length) return layoutLaneGraph(data, diagram, boxes, links);
    return layoutLayeredDiagram("graph", boxes, links, diagram.title);
  }

  var LANES = { margin: 28, header: 150, titleWidth: 104, pad: 26, colGap: 72, maxColGap: 190, rowGap: 16, bandGap: 44, portGap: 14, gapPad: 8 };

  // laneTitleLines wraps a lane title as the page draws it: upper case and
  // letter-spaced, so it is measured in capitals with room to spare.
  function laneTitleLines(title) {
    return wrapText(String(title || "").toUpperCase(), LANES.titleWidth, 11.5, 5);
  }

  // freeSpot returns the point nearest want inside one of the free intervals
  // left between boxes along an axis, within [lo, hi].
  function freeSpot(want, boxes, axis, lo, hi, pad) {
    var spans = boxes
      .map(function (b) {
        return axis === "x" ? [b.x - pad, b.x + b.width + pad] : [b.y - pad, b.y + b.height + pad];
      })
      .sort(function (a, b) {
        return a[0] - b[0];
      });
    var gaps = [];
    var at = lo;
    spans.forEach(function (sp) {
      if (sp[0] > at) gaps.push([at, sp[0]]);
      at = Math.max(at, sp[1]);
    });
    if (hi > at) gaps.push([at, hi]);
    var best = null;
    gaps.forEach(function (g) {
      var v = Math.min(Math.max(want, g[0]), g[1]);
      if (best === null || Math.abs(v - want) < Math.abs(best - want)) best = v;
    });
    return best === null ? want : best;
  }

  // layoutLaneGraph draws a graph with lanes as horizontal bands. Each lane
  // flows left to right by the order of its own nodes, in columns shared by
  // every lane, so the gaps between columns and between bands are clear
  // streets: an edge between lanes leaves its band straight, travels along
  // the streets, and enters its target's band straight.
  function layoutLaneGraph(data, diagram, boxes, links) {
    var g = LANES;
    var byId = dict();
    boxes.forEach(function (b) {
      byId[b.id] = b;
    });
    var laneDefs = list(diagram.lanes).map(function (l) {
      return { id: l.id, title: l.title || l.id };
    });
    var laneIndex = dict();
    laneDefs.forEach(function (l, i) {
      laneIndex[l.id] = i;
    });
    var laneOfNode = dict();
    list(diagram.nodes).forEach(function (n) {
      laneOfNode[n.id] = laneIndex[n.lane];
    });
    if (boxes.some(function (b) { return laneOfNode[b.id] === undefined; })) {
      laneDefs.push({ id: "", title: "Other" });
      boxes.forEach(function (b) {
        if (laneOfNode[b.id] === undefined) laneOfNode[b.id] = laneDefs.length - 1;
      });
    }
    // Ranks within each lane, from the edges inside it. A cycle is broken
    // where the author's order of nodes starts it.
    var rank = dict();
    laneDefs.forEach(function (def, li) {
      var ids = boxes
        .filter(function (b) {
          return laneOfNode[b.id] === li;
        })
        .map(function (b) {
          return b.id;
        });
      var inside = [];
      links.forEach(function (l, index) {
        if (laneOfNode[l.from] === li && laneOfNode[l.to] === li) inside.push({ from: l.from, to: l.to, index: index, self: l.from === l.to });
      });
      var r = rankEdges(ids, inside);
      ids.forEach(function (id) {
        rank[id] = r[id];
      });
    });
    var maxRank = 0;
    boxes.forEach(function (b) {
      maxRank = Math.max(maxRank, rank[b.id]);
    });
    var cells = laneDefs.map(function () {
      var row = [];
      for (var r = 0; r <= maxRank; r++) row.push([]);
      return row;
    });
    boxes
      .slice()
      .sort(function (a, b) {
        return byString(a.title, b.title) || byString(a.id, b.id);
      })
      .forEach(function (b) {
        cells[laneOfNode[b.id]][rank[b.id]].push(b);
      });
    // The gap between columns holds the labels of the edges that cross it.
    var colGap = g.colGap;
    links.forEach(function (l) {
      if (l.label && laneOfNode[l.from] === laneOfNode[l.to] && rank[l.to] > rank[l.from]) {
        colGap = Math.max(colGap, Math.min(labelWidth(wrapText(l.label, 150, LABEL_SIZE, 3)) + 28, g.maxColGap));
      }
    });
    var colWidth = [];
    var colX = [];
    var x = g.margin + g.header;
    for (var c = 0; c <= maxRank; c++) {
      colWidth[c] = 0;
      cells.forEach(function (row) {
        row[c].forEach(function (b) {
          colWidth[c] = Math.max(colWidth[c], b.width);
        });
      });
      colX[c] = x;
      x += colWidth[c] + colGap;
    }
    var right = x - colGap + g.pad;
    var y = g.margin;
    var lanes = laneDefs.map(function (def, li) {
      var tallest = 0;
      cells[li].forEach(function (cell) {
        tallest = Math.max(tallest, cell.reduce(function (h, b, k) {
          return h + b.height + (k ? g.rowGap : 0);
        }, 0));
      });
      var titleLines = laneTitleLines(def.title).length;
      var height = Math.max(tallest + 2 * g.pad, titleLines * 15 + 30);
      cells[li].forEach(function (cell, cc) {
        var h = cell.reduce(function (sum, b, k) {
          return sum + b.height + (k ? g.rowGap : 0);
        }, 0);
        var cy = y + (height - h) / 2;
        cell.forEach(function (b) {
          b.x = round(colX[cc] + (colWidth[cc] - b.width) / 2);
          b.y = round(cy);
          cy += b.height + g.rowGap;
        });
      });
      var lane = { id: def.id || "~other", title: def.title, x: g.margin, y: round(y), width: round(right - g.margin), height: round(height), nodes: [] };
      cells[li].forEach(function (cell) {
        cell.forEach(function (b) {
          lane.nodes.push(b.id);
        });
      });
      y += height + g.bandGap;
      return lane;
    });
    function bandOf(id) {
      return lanes[laneOfNode[id]];
    }
    function bandBoxes(li) {
      var out = [];
      cells[li].forEach(function (cell) {
        out = out.concat(cell);
      });
      return out;
    }
    // Decide each edge's sides, then spread the edges sharing a side.
    var routes = [];
    links
      .slice()
      .sort(function (a, b) {
        return byString(linkKey(a), linkKey(b));
      })
      .forEach(function (link) {
        var a = byId[link.from];
        var b = byId[link.to];
        var la = laneOfNode[a.id];
        var lb = laneOfNode[b.id];
        var kind;
        var sides;
        if (a === b) {
          kind = "self";
          sides = ["right", "right"];
        } else if (la === lb && rank[b.id] > rank[a.id]) {
          kind = "along";
          sides = ["right", "left"];
        } else if (la === lb) {
          kind = "back";
          sides = ["bottom", "bottom"];
        } else {
          kind = la < lb ? "down" : "up";
          sides = la < lb ? ["bottom", "top"] : ["top", "bottom"];
        }
        routes.push({ link: link, a: a, b: b, kind: kind, sides: sides });
      });
    var slots = dict();
    routes.forEach(function (r, i) {
      if (r.kind === "self") return;
      [[r.a, r.sides[0], r.b], [r.b, r.sides[1], r.a]].forEach(function (end, k) {
        var key = end[0].id + "|" + end[1];
        var vertical = end[1] === "left" || end[1] === "right";
        var other = end[2];
        (slots[key] = slots[key] || { box: end[0], side: end[1], entries: [] }).entries.push({
          route: r,
          end: k,
          order: vertical ? other.y + other.height / 2 : other.x + other.width / 2,
          index: i,
        });
      });
    });
    Object.keys(slots)
      .sort(byString)
      .forEach(function (key) {
        var sl = slots[key];
        sl.entries.sort(function (p, q) {
          return p.order - q.order || p.index - q.index || p.end - q.end;
        });
        var vertical = sl.side === "left" || sl.side === "right";
        var length = vertical ? sl.box.height : sl.box.width;
        var start = vertical ? sl.box.y : sl.box.x;
        var n = sl.entries.length;
        var spacing = n > 1 ? Math.min(g.portGap, (length - 12) / (n - 1)) : 0;
        var first = start + length / 2 - (spacing * (n - 1)) / 2;
        sl.entries.forEach(function (entry, i) {
          var p = sidePoint(sl.box, sl.side, first + i * spacing);
          entry.route[entry.end === 0 ? "p1" : "p2"] = { x: round(p.x), y: round(p.y) };
        });
      });
    var edges = routes.map(function (r) {
      var link = r.link;
      if (r.kind === "self") {
        return layeredEdge({ self: true, link: link, points: [
          { x: round(r.a.x + r.a.width), y: round(r.a.y + r.a.height * 0.3) },
          { x: round(r.a.x + r.a.width), y: round(r.a.y + r.a.height * 0.7) },
        ] }, link.from, link.to);
      }
      var p1 = r.p1;
      var p2 = r.p2;
      var band = bandOf(r.a.id);
      var points = [];
      var axis = "v";
      var label;
      if (r.kind === "along") {
        axis = "h";
        for (var cc = rank[r.a.id] + 1; cc < rank[r.b.id]; cc++) {
          var cx = colX[cc];
          var t = (cx - p1.x) / (p2.x - p1.x || 1);
          var yy = freeSpot(p1.y + (p2.y - p1.y) * t, cells[laneOfNode[r.a.id]][cc], "y", band.y + 6, band.y + band.height - 6, g.gapPad);
          points.push({ x: round(cx), y: round(yy) }, { x: round(cx + colWidth[cc]), y: round(yy) });
        }
        var next = points[0] || p2;
        label = { x: (p1.x + next.x) / 2, y: (p1.y + next.y) / 2 };
      } else if (r.kind === "back") {
        var corridor = round(band.y + band.height - g.pad / 2);
        points = [{ x: p1.x, y: corridor }, { x: p2.x, y: corridor }];
        axis = "h";
        label = { x: (p1.x + p2.x) / 2, y: corridor };
      } else {
        var down = r.kind === "down";
        var from = laneOfNode[r.a.id];
        var to = laneOfNode[r.b.id];
        var bandA = lanes[from];
        var bandB = lanes[to];
        points.push({ x: p1.x, y: round(down ? bandA.y + bandA.height : bandA.y) });
        var step = down ? 1 : -1;
        var count = Math.abs(to - from);
        for (var l = from + step, k = 1; l !== to; l += step, k++) {
          var mid = lanes[l];
          var want = p1.x + ((p2.x - p1.x) * k) / count;
          var fx = round(freeSpot(want, bandBoxes(l), "x", mid.x + g.header, mid.x + mid.width, g.gapPad));
          var top = { x: fx, y: round(mid.y) };
          var bottom = { x: fx, y: round(mid.y + mid.height) };
          if (down) points.push(top, bottom);
          else points.push(bottom, top);
        }
        points.push({ x: p2.x, y: round(down ? bandB.y : bandB.y + bandB.height) });
        label = { x: (points[0].x + points[1].x) / 2, y: (points[0].y + points[1].y) / 2 };
      }
      var path = curvePath(p1, NORMALS[r.sides[0]], p2, NORMALS[r.sides[1]], points, axis);
      var lines = link.label ? wrapText(link.label, 150, LABEL_SIZE, 3) : [];
      return {
        id: link.from + ">" + link.to + ">",
        from: link.from,
        to: link.to,
        via: "",
        label: link.label || "",
        labelLines: lines,
        labelWidth: labelWidth(lines),
        change: "",
        faded: false,
        refs: list(link.refs),
        x1: p1.x,
        y1: p1.y,
        x2: p2.x,
        y2: p2.y,
        d: path.d,
        labelX: round(label.x),
        labelY: round(label.y),
      };
    });
    return finishLayout({ kind: "graph", title: diagram.title, nodes: boxes, edges: edges, lanes: lanes, empty: null }, g.margin);
  }

  // layoutStateDiagram draws states top to bottom from the initial marker,
  // with every transition, loops and returns included.
  function layoutStateDiagram(data, diagram) {
    var known = dict();
    var boxes = list(diagram.states).map(function (st) {
      known[st.id] = true;
      var box = diagramBox(st.id, st.label || st.id, null, data);
      box.shape = "state";
      box.refs = list(st.refs);
      return box;
    });
    if (!boxes.length) return emptyLayout("state", "No states yet", "This state diagram lists no states.");
    var links = list(diagram.transitions)
      .filter(function (t) {
        return known[t.from] && known[t.to];
      })
      .map(function (t) {
        return { from: t.from, to: t.to, label: t.label || "", refs: list(t.refs), via: "" };
      });
    if (known[diagram.initial]) {
      boxes.push({ id: "~initial", ref: null, title: "", subtitle: "", kind: "", group: "", change: "", shape: "initial", x: 0, y: 0, width: 18, height: 18 });
      links.push({ from: "~initial", to: diagram.initial, label: "", refs: [], via: "" });
    }
    return layoutLayeredDiagram("state", boxes, links, diagram.title);
  }

  var ER = { margin: 32, header: 34, row: 24, colGap: 120, rowGap: 30, minWidth: 200, stubHeight: 34 };

  // layoutErDiagram draws an er diagram from the facts: a card per table
  // with its columns, parents left of the tables that reference them, and a
  // connector from each foreign-key column to the column it references.
  // Referenced tables the diagram does not list are faded stubs.
  function layoutErDiagram(data, diagram) {
    var e = ER;
    var db = null;
    data.facts.databases.forEach(function (candidate) {
      if (candidate && candidate.id === diagram.database) db = candidate;
    });
    if (!db) {
      return emptyLayout("er", "No tables for " + (diagram.database || "this database") + " yet", "Columns and keys come from the extracted facts, which do not list this database yet.");
    }
    var tables = dict();
    list(db.tables).forEach(function (t) {
      tables[t.name] = t;
    });
    var owner = db.package && data.nodeById["pkg:" + db.package] ? "pkg:" + db.package : null;
    var cards = dict();
    var names = [];
    function card(name, stub) {
      if (cards[name]) return cards[name];
      var table = tables[name];
      var fkCols = dict();
      list(table && table.foreignKeys).forEach(function (fk) {
        list(fk.columns).forEach(function (c) {
          fkCols[c] = true;
        });
      });
      var columns = stub || !table
        ? []
        : list(table.columns).map(function (c, i) {
            return {
              name: c.name,
              type: c.type || "",
              primaryKey: c.primaryKey || 0,
              foreignKey: !!fkCols[c.name],
              notNull: !!c.notNull,
              y: e.header + i * e.row,
              height: e.row,
            };
          });
      // Names and types are drawn in a monospace face.
      var caption = stub ? "not in this diagram" : db.id;
      var width = Math.max(e.minWidth, 34 + monoWidth(name, 13) + 18 + textWidth(caption, 10.5) + 14);
      columns.forEach(function (c) {
        width = Math.max(width, 56 + monoWidth(c.name, 12) + 16 + monoWidth((c.type || "ANY") + "?", 11) + 12);
      });
      var missing = !stub && !table;
      var height = stub ? e.stubHeight : e.header + Math.max(columns.length, missing ? 1 : 0) * e.row + 8;
      cards[name] = {
        id: name,
        ref: owner,
        title: name,
        subtitle: db.id,
        kind: "table",
        group: "",
        change: "",
        shape: "table",
        stub: !!stub,
        faded: !!stub,
        missing: missing,
        columns: columns,
        x: 0,
        y: 0,
        width: round(width),
        height: round(height),
      };
      names.push(name);
      return cards[name];
    }
    list(diagram.tables).forEach(function (name) {
      card(name, false);
    });
    var listed = names.slice();
    var relations = [];
    listed.forEach(function (name) {
      var table = tables[name];
      list(table && table.foreignKeys).forEach(function (fk) {
        var parent = card(fk.table, listed.indexOf(fk.table) < 0);
        var refs = list(fk.references);
        list(fk.columns).forEach(function (col, i) {
          var target = refs[i];
          if (!target && parent.columns.length) {
            var pk = parent.columns.filter(function (c) {
              return c.primaryKey;
            })[i];
            target = pk ? pk.name : "";
          }
          relations.push({ from: name, to: parent.id, fromColumn: col, toColumn: target || "" });
        });
      });
    });
    var ids = names.slice().sort(byString);
    var rankInput = relations.map(function (r, index) {
      return { from: r.to, to: r.from, index: index, self: r.from === r.to };
    });
    var rank = rankEdges(ids, rankInput);
    var maxRank = 0;
    ids.forEach(function (id) {
      maxRank = Math.max(maxRank, rank[id]);
    });
    var cols = [];
    for (var r = 0; r <= maxRank; r++) cols.push([]);
    ids.forEach(function (id) {
      cols[rank[id]].push(cards[id]);
    });
    var x = e.margin;
    var tallest = 0;
    var heights = cols.map(function (col) {
      col.sort(function (a, b) {
        return (a.stub ? 1 : 0) - (b.stub ? 1 : 0) || byString(a.id, b.id);
      });
      var h = col.reduce(function (sum, c, k) {
        return sum + c.height + (k ? e.rowGap : 0);
      }, 0);
      tallest = Math.max(tallest, h);
      return h;
    });
    var columns = cols.map(function (col, ci) {
      var width = Math.max.apply(null, col.map(function (c) { return c.width; }).concat([0]));
      var y = e.margin + (tallest - heights[ci]) / 2;
      col.forEach(function (c) {
        c.x = round(x + (width - c.width) / 2);
        c.y = round(y);
        y += c.height + e.rowGap;
      });
      var column = { x: x, width: width, y: e.margin, height: tallest, boxes: col };
      x += width + e.colGap;
      return column;
    });
    function rowY(c, name) {
      var col = c.columns.filter(function (k) {
        return k.name === name;
      })[0];
      return col ? c.y + col.y + col.height / 2 : c.y + Math.min(c.height, e.header) / 2;
    }
    var edges = relations.map(function (rel) {
      var child = cards[rel.from];
      var parent = cards[rel.to];
      var p1;
      var p2;
      var d;
      var extent = [];
      if (child === parent) {
        var sx = child.x + child.width;
        p1 = { x: round(sx), y: round(rowY(child, rel.fromColumn)) };
        p2 = { x: round(sx), y: round(rowY(parent, rel.toColumn)) };
        d = "M" + pt(p1) + " C" + pt({ x: sx + 44, y: p1.y }) + " " + pt({ x: sx + 44, y: p2.y }) + " " + pt(p2);
        extent.push({ x: sx + 48, y: Math.max(p1.y, p2.y) });
      } else {
        var sides = facingSides(child, parent);
        if (sides[0] === "top" || sides[0] === "bottom") sides = ["right", "right"];
        p1 = sidePoint(child, sides[0], rowY(child, rel.fromColumn));
        p2 = sidePoint(parent, sides[1], rowY(parent, rel.toColumn));
        p1 = { x: round(p1.x), y: round(p1.y) };
        p2 = { x: round(p2.x), y: round(p2.y) };
        var path = curvePath(p1, NORMALS[sides[0]], p2, NORMALS[sides[1]], passThrough(columns, p1, p2, 8), "h");
        d = path.d;
      }
      return {
        id: rel.from + "." + rel.fromColumn + ">" + rel.to + "." + rel.toColumn,
        from: rel.from,
        to: rel.to,
        fromColumn: rel.fromColumn,
        toColumn: rel.toColumn,
        via: "sql",
        label: "",
        labelLines: [],
        labelWidth: 0,
        change: "",
        faded: !!parent.stub,
        refs: [],
        x1: p1.x,
        y1: p1.y,
        x2: p2.x,
        y2: p2.y,
        d: d,
        extent: extent,
      };
    });
    edges.sort(function (a, b) {
      return byString(a.id, b.id);
    });
    var nodes = ids.map(function (id) {
      return cards[id];
    });
    return finishLayout({ kind: "er", title: diagram.title, database: db.id, nodes: nodes, edges: edges, lanes: [], empty: null }, e.margin);
  }

  // layoutDiagram draws one of a node's diagrams by its kind.
  function layoutDiagram(data, diagram) {
    switch (diagram && diagram.kind) {
      case "graph":
        return layoutGraphDiagram(data, diagram);
      case "sequence":
        return layoutSequence(data, diagram);
      case "state":
        return layoutStateDiagram(data, diagram);
      case "er":
        return layoutErDiagram(data, diagram);
    }
    return emptyLayout("diagram", "Cannot draw this diagram", "Its kind is not one the explorer knows.");
  }

  function titleOf(data, id) {
    var node = data.nodeById[id];
    return node && node.title ? node.title : packagePath(id) || id;
  }

  function relation(data, id, link) {
    return {
      id: id,
      path: packagePath(id),
      title: titleOf(data, id),
      label: link.label || "",
      via: link.via || "",
      refs: list(link.refs),
      change: link.change || "",
    };
  }

  function byRelation(a, b) {
    return byString(a.id, b.id) || byString(a.via, b.via);
  }

  // involvedFlows lists the flows a node takes part in, with the numbers of
  // the steps it sends or receives.
  function involvedFlows(data, id) {
    return data.flows
      .filter(function (f) {
        return list(f.participants).indexOf(id) >= 0;
      })
      .map(function (f) {
        var steps = [];
        list(f.steps).forEach(function (s, i) {
          if (s.from === id || s.to === id) steps.push(i + 1);
        });
        return { id: f.id, title: f.title || f.id, steps: steps, change: f.change || "" };
      })
      .sort(byTitleThenId);
  }

  function proposalById(data, id) {
    for (var i = 0; i < data.proposals.length; i++) {
      if (data.proposals[i].id === id) return data.proposals[i];
    }
    return null;
  }

  // nodeDetails gathers what the panel shows for one node: its package
  // facts, callers and dependencies from the import graph, its other
  // connections, glossary terms with definitions, the flows it takes part
  // in, its diagrams, what it contains and what contains it, every extracted
  // fact that names its package, what an active proposal changes, and the
  // History entries that touched its package, their commits linked by links
  // (pageLinks).
  function nodeDetails(data, id, links) {
    var node = data.nodeById[id] || null;
    var path = packagePath(id);
    var callers = [];
    var dependencies = [];
    var incoming = [];
    var outgoing = [];
    data.links.forEach(function (l) {
      if (l.from === l.to) return;
      var imports = l.via === "import";
      if (l.to === id) (imports ? callers : incoming).push(relation(data, l.from, l));
      if (l.from === id) (imports ? dependencies : outgoing).push(relation(data, l.to, l));
    });
    [callers, dependencies, incoming, outgoing].forEach(function (rs) {
      rs.sort(byRelation);
    });
    var proposal = node && node.proposal ? proposalById(data, node.proposal) : null;
    return {
      node: node,
      path: path,
      pkg: path ? data.packageByPath[path] || null : null,
      callers: callers,
      dependencies: dependencies,
      incoming: incoming,
      outgoing: outgoing,
      terms: node ? nodeTerms(data, node) : [],
      facts: path ? factsForPackage(data.facts, path) : [],
      flows: involvedFlows(data, id),
      diagrams: node
        ? node.diagrams.map(function (g) {
            return { id: g.id, title: g.title || g.id, kind: g.kind, summary: g.summary || null };
          })
        : [],
      contains: node
        ? node.contains
            .filter(function (c) {
              return !!data.nodeById[c];
            })
            .map(function (c) {
              return { id: c, title: titleOf(data, c) };
            })
        : [],
      containedBy: data.nodes
        .filter(function (n) {
          return n.contains.indexOf(id) >= 0;
        })
        .map(function (n) {
          return { id: n.id, title: n.title || n.id };
        }),
      change: node ? node.change || "" : "",
      changed: node ? list(node.changed) : [],
      proposal: proposal ? { id: proposal.id, title: proposal.title || proposal.id } : null,
      history: path ? packageHistory(data, path, links) : [],
    };
  }

  // proposalDetails gathers what the panel shows for a design proposal: its
  // statements and one line per operation, each with the node or flow to
  // jump to.
  function proposalDetails(data, id) {
    var p = proposalById(data, id);
    if (!p) return null;
    var titles = dict();
    Object.keys(data.nodeById).forEach(function (k) {
      titles[k] = titleOf(data, k);
    });
    list(p.nodes).forEach(function (op) {
      if (op.op === "add" && op.id) titles[op.id] = op.title || op.id;
    });
    var flowTitles = dict();
    data.flows.forEach(function (f) {
      flowTitles[f.id] = f.title || f.id;
    });
    var operations = [];
    list(p.nodes).forEach(function (op) {
      operations.push({ what: "node", op: op.op, label: titles[op.id] || op.id, target: op.id });
    });
    list(p.edges).forEach(function (op) {
      operations.push({
        what: "edge",
        op: op.op,
        label: (titles[op.from] || op.from) + " → " + (titles[op.to] || op.to),
        via: op.via || "",
        target: op.from,
      });
    });
    list(p.flows).forEach(function (op) {
      var label = op.op === "add" ? op.title || op.id : flowTitles[op.id] || op.id;
      operations.push({ what: "flow", op: op.op, label: label, target: "flow:" + op.id });
    });
    return {
      id: p.id,
      title: p.title || p.id,
      feature: p.feature || "",
      status: p.status || "",
      spec: p.spec || "",
      summary: p.summary || null,
      requirements: list(p.requirements),
      decisions: list(p.decisions),
      operations: operations,
    };
  }

  // nodeHome is where a node is drawn: a component in its focus view, a
  // context or container node in its level's view.
  function nodeHome(node) {
    if (node.level === "context") return { view: "context", node: node.id };
    if (node.level === "container") return { view: "containers", node: node.id };
    return { view: "focus", of: node.id, node: node.id };
  }

  function matchScore(text, q) {
    var s = String(text || "").toLowerCase();
    if (!s) return 0;
    if (s === q) return 100;
    if (s.indexOf(q) === 0) return 80;
    var at = s.indexOf(q);
    if (at < 0) return 0;
    return /[\s\/._:-]/.test(s.charAt(at - 1)) ? 60 : 40;
  }

  var SEARCH_ORDER = { node: 0, term: 1, flow: 2, diagram: 3, history: 4 };

  // search finds nodes by title or package path, glossary terms by term or
  // avoided synonym, flows, and diagrams by title, and History entries by
  // title, branch, or text; best matches first, each with the address state
  // that shows it.
  function search(data, query, limit) {
    var q = String(query || "").trim().toLowerCase();
    if (!q) return [];
    var results = [];
    function add(score, result) {
      if (score > 0) {
        result.score = score;
        results.push(result);
      }
    }
    data.nodes.forEach(function (node) {
      var path = packagePath(node.id);
      var score = Math.max(matchScore(node.title, q), 0.9 * matchScore(path || node.id, q), 0.9 * matchScore(node.id, q));
      add(score, { type: "node", id: node.id, label: node.title || node.id, detail: path || kindLabel(node.kind), state: nodeHome(node), change: node.change || "" });
    });
    data.facts.glossary.forEach(function (t) {
      var avoid = list(t.avoid);
      var bySynonym = 0;
      avoid.forEach(function (a) {
        bySynonym = Math.max(bySynonym, 0.6 * matchScore(a, q));
      });
      var direct = 0.95 * matchScore(t.term, q);
      var label = direct >= bySynonym || !avoid.length ? t.term : t.term + " (not " + avoid.join(", ") + ")";
      add(Math.max(direct, bySynonym), { type: "term", id: t.term, label: label, detail: t.definition || "", state: { term: t.term } });
    });
    data.flows.forEach(function (f) {
      add(0.9 * matchScore(f.title || f.id, q), { type: "flow", id: f.id, label: f.title || f.id, detail: "Flow", state: { view: "flow", of: f.id }, change: f.change || "" });
    });
    data.nodes.forEach(function (node) {
      node.diagrams.forEach(function (g) {
        add(0.85 * matchScore(g.title || g.id, q), {
          type: "diagram",
          id: g.id,
          label: g.title || g.id,
          detail: (node.title || node.id) + " diagram",
          state: { view: "diagram", of: g.id, node: node.id },
        });
      });
    });
    historyEntries(data).forEach(function (e) {
      var score = Math.max(0.85 * matchScore(e.title, q), 0.7 * matchScore(e.branch, q), 0.5 * matchScore(e.text.join(" "), q));
      add(score, { type: "history", id: e.id, label: e.title, detail: e.dateLabel + " · " + e.branch, state: { view: "history", entry: e.id } });
    });
    results.sort(function (a, b) {
      return b.score - a.score || SEARCH_ORDER[a.type] - SEARCH_ORDER[b.type] || byString(a.label.toLowerCase(), b.label.toLowerCase()) || byString(a.id, b.id);
    });
    return results.slice(0, limit || 12).map(function (r) {
      delete r.score;
      return r;
    });
  }

  // --------------------------------------------------------------- history
  //
  // History is CHANGELOG.md's entries, extracted into the facts: one per
  // merge to master, or per run of direct-to-master commits. Commits,
  // tickets, and pull requests link to the repository on GitHub, from disk
  // too, since they live nowhere else.

  // githubHref is the one place that builds links into the repository on
  // GitHub, the one links (pageLinks) names.
  function githubHref(path, links) {
    return (links || DISK_LINKS).repository + "/" + path;
  }

  var MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];
  var ISO_DATE = /^([0-9]{4})-([0-9]{2})-([0-9]{2})$/;

  // commitLink is a commit SHA and its page on GitHub; a value that is not
  // a SHA gets no link.
  function commitLink(sha, links) {
    if (!sha) return null;
    return { sha: sha, href: COMMIT.test(sha) ? githubHref("commit/" + sha, links) : null };
  }

  var ISSUE_NUMBER = /^#([1-9][0-9]*)$/;

  // resolveSource turns a History source into what the page shows: a ticket
  // or pull-request number opens on GitHub (whose issue page forwards to a
  // pull request), and anything else resolves as a reference; links
  // (pageLinks) says where both point.
  function resolveSource(source, data, links) {
    var issue = ISSUE_NUMBER.exec(source);
    if (issue) return { kind: "issue", text: source, href: githubHref("issues/" + issue[1], links) };
    return resolveRef(source, data, links);
  }

  function historySlug(text) {
    return String(text || "")
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "");
  }

  function historyMonthLabel(key) {
    var m = /^([0-9]{4})-([0-9]{2})$/.exec(key);
    return m && MONTHS[Number(m[2]) - 1] ? MONTHS[Number(m[2]) - 1] + " " + m[1] : "Undated";
  }

  // historyEntries lists the History entries newest first, entries of one
  // day in their written order, each with an id for the address (its date
  // and title), its commit links (githubHref), and its touched paths with
  // the package node each still has, or null once the package is gone.
  function historyEntries(data, links) {
    var used = dict();
    var entries = data.facts.history.map(function (raw, index) {
      var date = typeof raw.date === "string" ? raw.date : "";
      var parts = ISO_DATE.exec(date);
      var month = parts && MONTHS[Number(parts[2]) - 1] ? parts[1] + "-" + parts[2] : "undated";
      var base = historySlug((parts ? date + " " : "") + (raw.title || "")) || "entry";
      var id = base;
      for (var n = 2; used[id]; n++) id = base + "-" + n;
      used[id] = true;
      var range = raw.commits && raw.commits.from ? { from: commitLink(raw.commits.from, links), to: commitLink(raw.commits.to, links) } : null;
      return {
        order: index,
        sortKey: month === "undated" ? "" : date,
        entry: {
          id: id,
          date: date,
          dateLabel: month === "undated" ? date || "Undated" : MONTHS[Number(parts[2]) - 1].slice(0, 3) + " " + Number(parts[3]) + ", " + parts[1],
          month: month,
          title: raw.title || id,
          branch: raw.branch || "",
          merge: commitLink(raw.merge, links),
          commits: range,
          text: list(raw.text),
          sources: list(raw.sources),
          touches: list(raw.touches).map(function (path) {
            var node = "pkg:" + path;
            return { path: path, node: data.nodeById[node] ? node : null };
          }),
        },
      };
    });
    entries.sort(function (a, b) {
      return byString(b.sortKey, a.sortKey) || a.order - b.order;
    });
    return entries.map(function (e) {
      return e.entry;
    });
  }

  // packageHistory lists the History entries whose touched paths include
  // the package's path exactly, newest first.
  function packageHistory(data, path, links) {
    return historyEntries(data, links).filter(function (e) {
      return e.touches.some(function (t) {
        return t.path === path;
      });
    });
  }

  function historyEntryById(data, id) {
    var found = null;
    historyEntries(data).forEach(function (e) {
      if (e.id === id) found = e;
    });
    return found;
  }

  // historyTimeline groups the History entries by month, newest first.
  function historyTimeline(data, links) {
    var months = [];
    historyEntries(data, links).forEach(function (e) {
      var last = months[months.length - 1];
      if (!last || last.key !== e.month) {
        last = { key: e.month, label: historyMonthLabel(e.month), entries: [] };
        months.push(last);
      }
      last.entries.push(e);
    });
    return months;
  }

  // ------------------------------------------------------------ navigation

  var LEVELS = [
    { id: "context", label: "Context" },
    { id: "containers", label: "Containers" },
    { id: "components", label: "Components" },
    { id: "flows", label: "Flows" },
    { id: "history", label: "History" },
  ];

  var VIEW_LEVEL = {
    context: "context",
    containers: "containers",
    inside: "containers",
    components: "components",
    group: "components",
    focus: "components",
    flows: "flows",
    flow: "flows",
    diagram: "components",
    history: "history",
  };

  function levelOf(view) {
    return own(VIEW_LEVEL, view) || "context";
  }

  function flowById(data, id) {
    for (var i = 0; i < data.flows.length; i++) {
      if (data.flows[i].id === id) return data.flows[i];
    }
    return null;
  }

  function clampStep(step, steps) {
    var n = list(steps).length;
    if (!n) return undefined;
    return Math.min(Math.max(step || 1, 1), n);
  }

  // resolveState turns an address into a state the data can show: a view
  // whose subject exists, a selection that exists, a step inside its flow,
  // and only known proposals. Anything that cannot be shown falls back to
  // the view above it with a notice saying why.
  function resolveState(data, raw) {
    raw = raw || {};
    var state = {};
    var notice = "";
    var of = raw.of;
    switch (raw.view) {
      case undefined:
      case "":
      case "context":
      case "containers":
      case "components":
      case "flows":
        state.view = raw.view || "context";
        break;
      case "history":
        state.view = "history";
        if (raw.entry && historyEntryById(data, raw.entry)) state.entry = raw.entry;
        else if (raw.entry) notice = "There is no History entry " + raw.entry + ".";
        break;
      case "group":
        if (groupIndex(of) >= 0) {
          state.view = "group";
          state.of = of;
        } else {
          state.view = "components";
          notice = "There is no component group called " + of + ".";
        }
        break;
      case "focus":
        if (data.nodeById[of]) {
          state.view = "focus";
          state.of = of;
        } else {
          state.view = "components";
          notice = "There is no node " + of + " in the data.";
        }
        break;
      case "inside":
        if (data.nodeById[of] && data.nodeById[of].contains.length) {
          state.view = "inside";
          state.of = of;
        } else {
          state.view = "containers";
          notice = data.nodeById[of] ? data.nodeById[of].title + " does not list what it contains yet." : "There is no container " + of + " in the data.";
        }
        break;
      case "flow":
        var flow = flowById(data, of);
        if (flow) {
          state.view = "flow";
          state.of = of;
          state.step = clampStep(raw.step, flow.steps);
        } else {
          state.view = "flows";
          notice = "There is no flow " + of + " in the data.";
        }
        break;
      case "diagram":
        var found = findDiagram(data, of);
        if (found) {
          state.view = "diagram";
          state.of = of;
          if (found.diagram.kind === "sequence") state.step = clampStep(raw.step, found.diagram.steps);
        } else {
          state.view = "components";
          notice = "There is no diagram " + of + " in the data.";
        }
        break;
      default:
        state.view = "context";
        notice = "The explorer has no view called " + raw.view + ".";
    }
    if (state.step === undefined) delete state.step;
    if (raw.node && data.nodeById[raw.node]) state.node = raw.node;
    if (raw.term && data.termByName[raw.term]) state.term = raw.term;
    state.proposals = list(raw.proposals).filter(function (id) {
      return !!proposalById(data, id);
    });
    if (raw.panel === "proposals" && state.proposals.length) state.panel = "proposals";
    if (notice) state.notice = notice;
    return state;
  }

  function homeCrumbs(data, node) {
    if (node.level === "context") return [{ label: "Context", state: { view: "context" } }];
    if (node.level === "container") {
      return [
        { label: "Context", state: { view: "context" } },
        { label: "Containers", state: { view: "containers" } },
      ];
    }
    var crumbs = [{ label: "Components", state: { view: "components" } }];
    if (groupIndex(node.group) >= 0) crumbs.push({ label: groupLabel(node.group), state: { view: "group", of: node.group } });
    return crumbs;
  }

  // breadcrumb lists the way back up from a state, each step with the state
  // it leads to; the last crumb is where the page is.
  function breadcrumb(data, state) {
    var crumbs;
    var node;
    switch (state.view) {
      case "containers":
        crumbs = [{ label: "Context", state: { view: "context" } }, { label: "Containers" }];
        break;
      case "inside":
        crumbs = [
          { label: "Context", state: { view: "context" } },
          { label: "Containers", state: { view: "containers" } },
          { label: titleOf(data, state.of) },
        ];
        break;
      case "components":
        crumbs = [{ label: "Components" }];
        break;
      case "group":
        crumbs = [{ label: "Components", state: { view: "components" } }, { label: groupLabel(state.of) }];
        break;
      case "focus":
        node = data.nodeById[state.of];
        crumbs = homeCrumbs(data, node).concat([{ label: node.title || node.id }]);
        break;
      case "flows":
        crumbs = [{ label: "Flows" }];
        break;
      case "flow":
        crumbs = [{ label: "Flows", state: { view: "flows" } }, { label: flowById(data, state.of).title || state.of }];
        break;
      case "history":
        crumbs = [{ label: "History" }];
        break;
      case "diagram":
        var found = findDiagram(data, state.of);
        node = found.node;
        crumbs = homeCrumbs(data, node).concat([
          { label: node.title || node.id, state: { view: "focus", of: node.id } },
          { label: found.diagram.title || found.diagram.id },
        ]);
        break;
      default:
        crumbs = [{ label: "Context" }];
    }
    return crumbs;
  }

  // layoutView draws whatever view a resolved state names; History's commits
  // link as links (pageLinks) says.
  function layoutView(data, state, links) {
    switch (state.view) {
      case "containers":
        return layoutContainers(data);
      case "inside":
        return layoutInside(data, state.of);
      case "components":
        return layoutGroupOverview(groupOverview(data));
      case "group":
        return layoutGroup(data, state.of);
      case "focus":
        return layoutFocus(data, state.of);
      case "flows":
        var items = flowIndex(data);
        return {
          kind: "flows",
          items: items,
          width: 0,
          height: 0,
          nodes: [],
          edges: [],
          lanes: [],
          empty: items.length ? null : { title: "No flows yet", text: "Key runtime flows appear here as content files describe them, each as a sequence diagram you can step through." },
        };
      case "flow":
        return layoutSequence(data, flowById(data, state.of));
      case "history":
        var months = historyTimeline(data, links);
        return {
          kind: "history",
          months: months,
          width: 0,
          height: 0,
          nodes: [],
          edges: [],
          lanes: [],
          empty: months.length ? null : { title: "No History yet", text: "Each merge to master appears here once CHANGELOG.md records it." },
        };
      case "diagram":
        return layoutDiagram(data, findDiagram(data, state.of).diagram);
    }
    return layoutContext(data);
  }

  // --------------------------------------------------------- viewport model
  //
  // The canvas shows a box of the drawing (an SVG viewBox) with the
  // canvas's own aspect, so zooming and panning are box arithmetic.

  // fitBox returns the view that shows rect whole, centred, with padding,
  // no larger than maxScale.
  function fitBox(rect, vw, vh, opts) {
    opts = opts || {};
    var pad = opts.pad || 0;
    var maxScale = opts.maxScale || 1.5;
    var w = Math.max(rect.width, 1) + 2 * pad;
    var h = Math.max(rect.height, 1) + 2 * pad;
    var scale = Math.min(vw / w, vh / h, maxScale);
    if (!(scale > 0)) scale = 1;
    var bw = vw / scale;
    var bh = vh / scale;
    return { x: rect.x + rect.width / 2 - bw / 2, y: rect.y + rect.height / 2 - bh / 2, width: bw, height: bh };
  }

  // zoomBox zooms the view by factor about the drawing point (fx, fy), so
  // that point stays where it is on the canvas; the scale (canvas width over
  // view width) stays within the limits.
  function zoomBox(box, factor, fx, fy, vw, limits) {
    limits = limits || {};
    var scale = vw / box.width;
    var next = Math.min(Math.max(scale * factor, limits.minScale || 0.05), limits.maxScale || 8);
    var k = scale / next;
    return {
      x: fx - (fx - box.x) * k,
      y: fy - (fy - box.y) * k,
      width: box.width * k,
      height: box.height * k,
    };
  }

  function lerpBox(a, b, t) {
    return {
      x: a.x + (b.x - a.x) * t,
      y: a.y + (b.y - a.y) * t,
      width: a.width + (b.width - a.width) * t,
      height: a.height + (b.height - a.height) * t,
    };
  }

  // ease is a smooth start and finish for view transitions.
  function ease(t) {
    return t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2;
  }

  // neighbours lists what hovering a drawn node lights: the node, the nodes
  // it shares an edge with, and those edges (by index).
  function neighbours(layout, id) {
    var nodes = dict();
    var edges = [];
    list(layout.edges).forEach(function (e, i) {
      if (e.from !== id && e.to !== id) return;
      nodes[e.from] = true;
      nodes[e.to] = true;
      edges.push(i);
    });
    var known = list(layout.nodes).some(function (n) {
      return n.id === id;
    });
    if (known) nodes[id] = true;
    return { nodes: Object.keys(nodes).sort(byString), edges: edges };
  }

  // termUsers lists the nodes that name a glossary term or cite it.
  function termUsers(data, term) {
    return data.nodes
      .filter(function (node) {
        return nodeTerms(data, node).some(function (t) {
          return t.term === term;
        });
      })
      .map(function (node) {
        return { id: node.id, title: node.title || node.id, kind: node.kind || "" };
      })
      .sort(byTitleThenId);
  }

  var CHANGE_WORDS = { added: "added", changed: "changed", removed: "removed" };

  // accessibleName says what a drawn node is, for screen readers and
  // keyboard users: its title, what kind of thing it is, and any change a
  // proposal makes to it.
  function accessibleName(n) {
    if (!n) return "";
    if (n.shape === "initial") return "Initial state";
    var parts = [n.title || n.id];
    var path = packagePath(n.ref || "");
    if (n.kind === "table") {
      parts.push("table in " + n.subtitle);
      if (n.stub) parts.push("not listed in this diagram");
      else if (n.missing) parts.push("not in the facts yet");
      else parts.push(n.columns.length + (n.columns.length === 1 ? " column" : " columns"));
    } else if (n.boundary) {
      parts.push(n.subtitle, "outside this view");
    } else if (n.shape === "state") {
      parts.push("state");
    } else if (path) {
      parts.push("package " + path);
    } else if (n.kind) {
      parts.push(kindLabel(n.kind));
    }
    if (own(CHANGE_WORDS, n.change)) parts.push(CHANGE_WORDS[n.change] + " by a design proposal");
    return parts.filter(Boolean).join(", ");
  }

  // packageDetails is the name the panel first used for nodeDetails.
  var packageDetails = nodeDetails;

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
  var NOT_PACKAGE_FACTS = ["format", "kind", "packages", "imports", "glossary", "adrs", "history"];

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
      var format = own(FACT_FORMATS, key) || genericFact;
      var items = [];
      facts[key].forEach(function (item) {
        if (item && item.package === path) items = items.concat(format(item));
      });
      if (items.length) sections.push({ key: key, title: capitalize(key), items: items });
    });
    return sections;
  }

  // servedRepository is the repository build.js names, or null without
  // usable build information.
  function servedRepository(build) {
    return build && typeof build.repository === "string" && REPOSITORY_URL.test(build.repository) ? build.repository : null;
  }

  // pageLinks is where the page's links point, given its protocol and what
  // build.js said about the running binary: root for code and doc links
  // (checkoutHref), repository for History's commits, tickets, and pull
  // requests (githubHref). From disk, code and doc links point into the
  // owner's checkout. Served, they open GitHub: the build's repository at
  // its commit, or at master when the build has no commit stamp or had
  // uncommitted changes; the public repository at master when build.js did
  // not load or names no usable repository.
  function pageLinks(protocol, build) {
    if (protocol === "file:") return DISK_LINKS;
    var repository = servedRepository(build);
    if (!repository) return { root: DEFAULT_REPOSITORY + "/blob/master/", repository: DEFAULT_REPOSITORY };
    var clean = typeof build.commit === "string" && COMMIT.test(build.commit) && build.modified !== true;
    return { root: repository + "/blob/" + (clean ? build.commit : "master") + "/", repository: repository };
  }

  // buildScript is the script that tells a served page about the running
  // binary; a page opened from disk has none.
  function buildScript(protocol) {
    return protocol === "file:" ? null : "build.js";
  }

  // isEmbedded reads the embedded flag Evie's frame puts in the address.
  function isEmbedded(search) {
    return String(search || "")
      .replace(/^\?/, "")
      .split("&")
      .indexOf("embedded=1") >= 0;
  }

  // checkoutHref links a repository path under the root links (pageLinks)
  // names, encoding each segment. A path that could leave the repository or
  // name a scheme gets no link.
  function checkoutHref(path, links) {
    if (!path || path.charAt(0) === "/" || path.indexOf(":") >= 0 || path.indexOf("\\") >= 0) return null;
    var segments = path.split("/");
    for (var i = 0; i < segments.length; i++) {
      if (segments[i] === "" || segments[i] === "." || segments[i] === "..") return null;
    }
    return (links || DISK_LINKS).root + segments.map(encodeURIComponent).join("/");
  }

  function splitAnchor(target) {
    var i = target.indexOf("#");
    return i < 0 ? { path: target, anchor: "" } : { path: target.slice(0, i), anchor: target.slice(i + 1) };
  }

  // ROUTE_REF is a route reference's one form, the same expression the
  // explorer tool's check accepts (routeRef in scripts/explorer/refs.go): a
  // pattern starting with "/", optionally after an upper-case method and one
  // space.
  var ROUTE_REF = /^(?:([A-Z]+) )?(\/.*)$/;

  // factOwner finds the node of the package that defines a table, route, or
  // command named by a reference.
  function factOwner(data, kind, target) {
    var pkg = null;
    if (kind === "table") {
      var dot = target.indexOf(".");
      var dbId = dot < 0 ? target : target.slice(0, dot);
      var table = dot < 0 ? "" : target.slice(dot + 1);
      data.facts.databases.forEach(function (db) {
        if (db.id !== dbId) return;
        list(db.tables).forEach(function (t) {
          if (t.name === table) pkg = db.package;
        });
      });
    } else if (kind === "route") {
      var route = ROUTE_REF.exec(target);
      var method = route ? route[1] || "" : "";
      data.facts.routes.forEach(function (r) {
        if (route && !pkg && r.pattern === route[2] && (!method || r.method === method)) pkg = r.package;
      });
    } else if (kind === "command") {
      var words = target.split(" ").filter(Boolean);
      data.facts.commands.forEach(function (c) {
        if (pkg || c.binary !== words[0]) return;
        var have = list(c.words);
        var match = words.slice(1).every(function (w, i) {
          return have[i] === w;
        });
        if (match && (words.length === 1 || have.length === words.length - 1)) pkg = c.package;
      });
    }
    var id = pkg ? "pkg:" + pkg : null;
    return id && data.nodeById[id] ? id : null;
  }

  // resolveRef turns a typed reference into what the page shows: text, a
  // link (checkoutHref) for code and doc references, a title for ADRs and
  // glossary terms, or an in-page target.
  function resolveRef(ref, data, links) {
    var i = typeof ref === "string" ? ref.indexOf(":") : -1;
    if (i <= 0) return { kind: "unknown", text: String(ref) };
    var kind = ref.slice(0, i);
    var target = ref.slice(i + 1);
    var out = { kind: kind, text: target };
    var href;
    switch (kind) {
      case "code":
        href = checkoutHref(splitAnchor(target).path, links);
        if (href) out.href = href;
        return out;
      case "doc":
        var parts = splitAnchor(target);
        href = checkoutHref(parts.path, links);
        if (href) out.href = href + (parts.anchor ? "#" + encodeURIComponent(parts.anchor) : "");
        return out;
      case "adr":
        var adr = data.adrByNumber[target];
        out.text = "ADR " + target + (adr ? ": " + adr.title : "");
        if (adr) {
          href = checkoutHref(adr.path, links);
          if (href) out.href = href;
          out.title = adr.title;
        }
        return out;
      case "term":
        var entry = data.termByName[target];
        if (entry) {
          out.title = entry.definition;
          out.definition = entry.definition;
        }
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
        var owner = factOwner(data, kind, target);
        if (owner) out.target.node = owner;
        return out;
    }
    return { kind: "unknown", text: ref };
  }

  // The address fragment holds the view and its subject ("of": a group, a
  // node, a flow, or a diagram), the selected node or glossary term, the
  // flow step, the selected History entry, the panel, and the active
  // proposals.
  var HASH_KEYS = ["view", "of", "node", "term", "step", "entry", "panel", "proposals"];

  function encodeValue(value) {
    return encodeURIComponent(value).replace(/%3A/gi, ":").replace(/%2F/gi, "/");
  }

  function formatHash(state) {
    var parts = [];
    HASH_KEYS.forEach(function (key) {
      var value = state ? state[key] : null;
      if (key === "proposals") {
        if (list(value).length) parts.push(key + "=" + list(value).map(encodeValue).join(","));
      } else if (value) {
        parts.push(key + "=" + encodeValue(String(value)));
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
        var raw = pair.slice(i + 1);
        if (HASH_KEYS.indexOf(key) < 0) return;
        try {
          if (key === "proposals") {
            var ids = [];
            raw.split(",").forEach(function (part) {
              var id = decodeURIComponent(part);
              if (id && ids.indexOf(id) < 0) ids.push(id);
            });
            if (ids.length) state.proposals = ids;
          } else if (key === "step") {
            if (/^[1-9][0-9]{0,5}$/.test(raw)) state.step = Number(raw);
          } else {
            var value = decodeURIComponent(raw);
            if (value) state[key] = value;
          }
        } catch (e) {
          // a malformed escape leaves the key unset
        }
      });
    return state;
  }

  // A proposal script path is relative to the explorer directory: plain
  // segments of letters, digits, dot, dash, and underscore, ending in .js.
  // Nothing absolute, no scheme, no "." or ".." segment, no escapes.
  var PROPOSAL_PATH = /^[A-Za-z0-9_-][A-Za-z0-9._-]*(\/[A-Za-z0-9_-][A-Za-z0-9._-]*)*\.js$/;

  // proposalPath reads the proposal query parameter from a location's search
  // string: null when there is none, {path} for a script the page may load,
  // and {error} for anything it refuses.
  function proposalPath(search) {
    var pairs = String(search || "")
      .replace(/^\?/, "")
      .split("&");
    for (var i = 0; i < pairs.length; i++) {
      var eq = pairs[i].indexOf("=");
      var key = eq < 0 ? pairs[i] : pairs[i].slice(0, eq);
      if (key !== "proposal") continue;
      var value;
      try {
        value = decodeURIComponent(eq < 0 ? "" : pairs[i].slice(eq + 1));
      } catch (e) {
        return { error: "The design proposal parameter is not a valid escape sequence." };
      }
      if (!PROPOSAL_PATH.test(value)) {
        return {
          error:
            "The design proposal parameter must be a relative path to a .js file inside the explorer directory, such as examples/design-proposal.js; absolute paths, schemes, and .. are refused.",
        };
      }
      return { path: value };
    }
    return null;
  }

  var model = {
    GROUPS: GROUPS,
    mergeData: mergeData,
    applyProposals: applyProposals,
    changeStyle: changeStyle,
    proposalLegend: proposalLegend,
    groupOverview: groupOverview,
    layoutGroupOverview: layoutGroupOverview,
    layoutContext: layoutContext,
    layoutContainers: layoutContainers,
    layoutGroup: layoutGroup,
    layoutInside: layoutInside,
    layoutFocus: layoutFocus,
    layoutSequence: layoutSequence,
    flowIndex: flowIndex,
    layoutDiagram: layoutDiagram,
    findDiagram: findDiagram,
    textWidth: textWidth,
    wrapText: wrapText,
    packageDetails: packageDetails,
    nodeDetails: nodeDetails,
    proposalDetails: proposalDetails,
    involvedFlows: involvedFlows,
    search: search,
    historyTimeline: historyTimeline,
    resolveSource: resolveSource,
    nodeHome: nodeHome,
    termUsers: termUsers,
    accessibleName: accessibleName,
    kindLabel: kindLabel,
    LEVELS: LEVELS,
    levelOf: levelOf,
    resolveState: resolveState,
    breadcrumb: breadcrumb,
    layoutView: layoutView,
    fitBox: fitBox,
    zoomBox: zoomBox,
    lerpBox: lerpBox,
    ease: ease,
    neighbours: neighbours,
    factsForPackage: factsForPackage,
    resolveRef: resolveRef,
    pageLinks: pageLinks,
    buildScript: buildScript,
    isEmbedded: isEmbedded,
    formatHash: formatHash,
    parseHash: parseHash,
    proposalPath: proposalPath,
  };

  if (typeof module !== "undefined" && module.exports) {
    module.exports = model;
  }

  if (typeof document === "undefined") return;

  // ------------------------------------------------------------- rendering
  //
  // Everything below draws the model into the page. Data only ever reaches
  // the page as text (textContent) or attribute values (setAttribute), never
  // as markup.

  var SVG_NS = "http://www.w3.org/2000/svg";

  function svg(name, attrs, parent) {
    var el = document.createElementNS(SVG_NS, name);
    Object.keys(attrs || {}).forEach(function (key) {
      if (attrs[key] !== undefined && attrs[key] !== null) el.setAttribute(key, String(attrs[key]));
    });
    if (parent) parent.appendChild(el);
    return el;
  }

  function html(name, attrs, parent, text) {
    var el = document.createElement(name);
    Object.keys(attrs || {}).forEach(function (key) {
      if (attrs[key] !== undefined && attrs[key] !== null && attrs[key] !== false) el.setAttribute(key, String(attrs[key]));
    });
    if (text !== undefined) el.textContent = text;
    if (parent) parent.appendChild(el);
    return el;
  }

  function byId(id) {
    return document.getElementById(id);
  }

  function clear(el) {
    while (el.firstChild) el.removeChild(el.firstChild);
  }

  // Icons are 16 by 16 stroked paths.
  var ICONS = {
    person: "M8 7.2a2.6 2.6 0 1 0 0-5.2 2.6 2.6 0 0 0 0 5.2zM2.8 14.2c.4-2.9 2.6-4.6 5.2-4.6s4.8 1.7 5.2 4.6",
    system: "M2.5 3.5h11v9h-11zM2.5 6h11M5 4.75h.01M7 4.75h.01",
    "external-system": "M8 1.8a6.2 6.2 0 1 0 0 12.4A6.2 6.2 0 0 0 8 1.8zM1.8 8h12.4M8 1.8c1.7 1.8 2.5 3.9 2.5 6.2S9.7 12.4 8 14.2M8 1.8C6.3 3.6 5.5 5.7 5.5 8s.8 4.4 2.5 6.2",
    process: "M2.5 2.5h11v4.5h-11zM2.5 9h11v4.5h-11zM5 4.75h.01M5 11.25h.01",
    "user-interface": "M2 3h12v10H2zM2 5.8h12M4.2 4.4h.01M5.8 4.4h.01",
    cli: "M2 3h12v10H2zM4.5 6.5l2 1.5-2 1.5M8 10h3.5",
    store: "M3 4c0-1.1 2.2-2 5-2s5 .9 5 2v8c0 1.1-2.2 2-5 2s-5-.9-5-2zM3 4c0 1.1 2.2 2 5 2s5-.9 5-2M3 8c0 1.1 2.2 2 5 2s5-.9 5-2",
    package: "M8 1.6l5.6 3v6.8L8 14.4l-5.6-3V4.6zM2.4 4.6L8 7.6l5.6-3M8 7.6v6.8",
    table: "M2 3h12v10H2zM2 6.3h12M2 9.6h12M6.5 6.3V13",
    state: "M3 5.5h10a2.5 2.5 0 0 1 0 5H3a2.5 2.5 0 0 1 0-5z",
    flow: "M2 4.5h9M8.5 2l2.5 2.5L8.5 7M14 11.5H5M7.5 9L5 11.5 7.5 14",
    diagram: "M2.5 2.5h4v4h-4zM9.5 9.5h4v4h-4zM6.5 4.5h3.5v5",
    search: "M7 2.5a4.5 4.5 0 1 0 0 9 4.5 4.5 0 0 0 0-9zM10.3 10.3L13.5 13.5",
    "zoom-in": "M7 2.5a4.5 4.5 0 1 0 0 9 4.5 4.5 0 0 0 0-9zM10.3 10.3L13.5 13.5M5 7h4M7 5v4",
    "zoom-out": "M7 2.5a4.5 4.5 0 1 0 0 9 4.5 4.5 0 0 0 0-9zM10.3 10.3L13.5 13.5M5 7h4",
    fit: "M2.5 6V2.5H6M10 2.5h3.5V6M13.5 10v3.5H10M6 13.5H2.5V10",
    close: "M4 4l8 8M12 4l-8 8",
    prev: "M10 3.5L5.5 8l4.5 4.5",
    next: "M6 3.5l4.5 4.5L6 12.5",
    code: "M5.5 4.5L2 8l3.5 3.5M10.5 4.5L14 8l-3.5 3.5",
    doc: "M4 1.8h5l3 3v9.4H4zM9 1.8v3h3M6 8h4M6 10.5h4",
    adr: "M8 1.8a6.2 6.2 0 1 0 0 12.4A6.2 6.2 0 0 0 8 1.8zM5.2 8.1l1.9 1.9 3.8-3.9",
    term: "M2.5 3h4a1.8 1.8 0 0 1 1.5.8A1.8 1.8 0 0 1 9.5 3h4v9.5h-4a1.5 1.5 0 0 0-1.5 1.2 1.5 1.5 0 0 0-1.5-1.2h-4z",
    route: "M2 8h10M9 4.5L12.5 8 9 11.5M2 4.5v7",
    command: "M2 3h12v10H2zM4.5 6.5l2 1.5-2 1.5M8 10h3.5",
    node: "M8 2.2l5 2.9v5.8L8 13.8l-5-2.9V5.1z",
    open: "M9.5 2.5h4v4M13.5 2.5L8 8M12 9.5v4H2.5V4h4",
    focus: "M8 5.5a2.5 2.5 0 1 0 0 5 2.5 2.5 0 0 0 0-5zM1.5 8h3M11.5 8h3",
    inside: "M2.5 2.5h11v11h-11zM5.5 5.5h5v5h-5z",
    group: "M2 2.5h5v5H2zM9 2.5h5v5H9zM2 9.5h5v4H2zM9 9.5h5v4H9z",
    history: "M8 1.8a6.2 6.2 0 1 0 0 12.4A6.2 6.2 0 0 0 8 1.8zM8 4.6V8l2.4 1.6",
    branch: "M4.5 2.5v11M4.5 5.5a3.5 3.5 0 0 0 3.5 3.5h1.5M11.5 6.5a2 2 0 1 0 0 4 2 2 0 0 0 0-4z",
    commit: "M8 5.6a2.4 2.4 0 1 0 0 4.8 2.4 2.4 0 0 0 0-4.8zM1.5 8h4.1M10.4 8h4.1",
    issue: "M8 1.8a6.2 6.2 0 1 0 0 12.4A6.2 6.2 0 0 0 8 1.8zM8 7.2a.8.8 0 1 0 0 1.6.8.8 0 0 0 0-1.6z",
  };

  function htmlIcon(name, parent, cls) {
    var el = svg("svg", { viewBox: "0 0 16 16", class: "icon" + (cls ? " " + cls : ""), "aria-hidden": "true", focusable: "false" }, parent);
    svg("path", { d: ICONS[name] || ICONS.node }, el);
    return el;
  }

  function svgIcon(name, parent, x, y, size) {
    var g = svg("g", { class: "glyph", transform: "translate(" + x + "," + y + ") scale(" + size / 16 + ")" }, parent);
    svg("path", { d: ICONS[name] || ICONS.node }, g);
    return g;
  }

  var app = {
    base: null,
    data: null,
    // Where links point (pageLinks); build.js names the build they open.
    links: pageLinks(location.protocol, null),
    raw: {},
    state: {},
    layout: null,
    viewKey: "",
    box: null,
    svg: null,
    content: null,
    nodeEls: dict(),
    edgeEls: [],
    problems: [],
    flash: null,
    expanded: dict(),
    overlays: dict(),
    animating: null,
  };

  var reduceMotion = typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  // Inside Evie's frame the page drops its brand and keeps the light theme;
  // explorer.css reads the attribute.
  if (isEmbedded(location.search)) document.documentElement.setAttribute("data-embedded", "");

  // -------------------------------------------------------- state changes

  function overlayFor(ids) {
    var key = list(ids).join(",");
    if (!app.overlays[key]) app.overlays[key] = applyProposals(app.base, ids);
    return app.overlays[key];
  }

  function writeHash(raw, push) {
    var hash = formatHash(raw);
    var url = location.pathname + location.search + hash;
    try {
      if (push) history.pushState(null, "", url);
      else history.replaceState(null, "", url);
    } catch (e) {
      // file:// pages in some browsers refuse history changes; the fragment
      // still works.
      if (location.hash !== hash) location.hash = hash;
    }
  }

  var DEPTH = { context: 0, containers: 1, components: 1, flows: 1, inside: 2, group: 2, flow: 2, focus: 3, diagram: 4 };

  // go moves the page to a new address. View changes add a history entry
  // and animate; selection and step changes replace the current one.
  function go(next, opts) {
    opts = opts || {};
    var raw = {};
    Object.keys(next).forEach(function (key) {
      if (next[key] !== undefined && next[key] !== "") raw[key] = next[key];
    });
    if (!("proposals" in next)) raw.proposals = list(app.raw.proposals);
    var viewChange = raw.view !== app.state.view || raw.of !== app.state.of || list(raw.proposals).join(",") !== list(app.state.proposals).join(",");
    writeHash(raw, viewChange && !opts.replace);
    apply(raw, opts);
  }

  // select changes only the selection, keeping the view.
  function select(id, extra) {
    var next = { view: app.state.view, of: app.state.of, step: app.state.step, node: id || undefined, proposals: app.state.proposals };
    go(copy(extra, next), { replace: true });
  }

  function apply(raw, opts) {
    opts = opts || {};
    app.raw = raw;
    app.data = overlayFor(raw.proposals);
    var previous = app.state;
    var state = resolveState(app.data, raw);
    app.state = state;
    var clean = copy(state);
    delete clean.notice;
    if (formatHash(clean) !== formatHash(raw)) writeHash(clean, false);
    var key = state.view + "|" + (state.of || "") + "|" + list(state.proposals).join(",");
    var changed = key !== app.viewKey;
    if (changed) {
      app.viewKey = key;
      app.layout = layoutView(app.data, state, app.links);
    }
    // The panel and bars first: they decide the canvas's size, which the
    // new view is fitted to.
    renderNotice(state.notice);
    renderLevels();
    renderCrumbs();
    renderStepper();
    renderRail();
    renderPanel();
    if (changed) showLayout(previous, state, opts);
    else refreshCanvasState();
  }

  // ------------------------------------------------------ viewport on screen

  // ZOOM_STEP is how far one press of a zoom button or key zooms in or out.
  var ZOOM_STEP = 1.3;

  function canvasSize() {
    var canvas = byId("canvas");
    return { w: Math.max(canvas.clientWidth, 200), h: Math.max(canvas.clientHeight, 200) };
  }

  function setBox(box) {
    app.box = box;
    app.size = canvasSize();
    if (app.svg) app.svg.setAttribute("viewBox", [box.x, box.y, box.width, box.height].map(function (n) { return round(n); }).join(" "));
    pinHeads();
  }

  function layoutRect(layout) {
    return { x: 0, y: 0, width: layout.width, height: layout.height };
  }

  // homeBox is where a view opens: the whole drawing, or for a long flow
  // the top at a readable scale.
  function homeBox(layout) {
    var size = canvasSize();
    if (layout.kind === "sequence") {
      var scale = Math.min(Math.max(size.w / (layout.width + 40), 0.88), 1.1);
      var w = size.w / scale;
      var h = size.h / scale;
      var x = layout.width + 40 <= w ? layout.width / 2 - w / 2 : -20;
      return { x: x, y: -8, width: w, height: h };
    }
    var fitted = fitBox(layoutRect(layout), size.w, size.h, { pad: 24, maxScale: 1.15 });
    if (layout.kind === "er" && size.w / fitted.width < 0.7) {
      // A large schema opens readable at its first tables; Fit shows it all.
      var left = Math.min.apply(null, layout.nodes.map(function (n) { return n.x; }));
      var top = Math.min.apply(null, layout.nodes.filter(function (n) { return n.x < left + 160; }).map(function (n) { return n.y; }));
      var k = 0.85;
      return { x: left - 40, y: Math.max(-20, top - 60), width: size.w / k, height: size.h / k };
    }
    return fitted;
  }

  function animateBox(target, ms, done) {
    if (app.animating) cancelAnimationFrame(app.animating);
    if (reduceMotion || !app.box) {
      setBox(target);
      if (done) done();
      return;
    }
    var from = app.box;
    var start = null;
    function frame(now) {
      if (start === null) start = now;
      var t = Math.min((now - start) / ms, 1);
      setBox(lerpBox(from, target, ease(t)));
      if (t < 1) app.animating = requestAnimationFrame(frame);
      else {
        app.animating = null;
        if (done) done();
      }
    }
    app.animating = requestAnimationFrame(frame);
  }

  function scaleBox(box, k) {
    var cx = box.x + box.width / 2;
    var cy = box.y + box.height / 2;
    return { x: cx - (box.width * k) / 2, y: cy - (box.height * k) / 2, width: box.width * k, height: box.height * k };
  }

  function rectOf(layout, id) {
    if (!layout || !id) return null;
    var found = null;
    list(layout.nodes).concat(list(layout.panels)).forEach(function (n) {
      if (!found && n.id === id) found = n;
    });
    if (!found && layout.kind === "sequence") {
      list(layout.participants).forEach(function (p) {
        if (!found && p.id === id) found = p;
      });
    }
    return found ? { x: found.x, y: found.y, width: found.width, height: found.height } : null;
  }

  function zoomBy(factor, fx, fy) {
    if (!app.box) return;
    var size = canvasSize();
    var cx = fx === undefined ? app.box.x + app.box.width / 2 : fx;
    var cy = fy === undefined ? app.box.y + app.box.height / 2 : fy;
    var next = zoomBox(app.box, factor, cx, cy, size.w, { minScale: 0.08, maxScale: 4 });
    if (fx === undefined) animateBox(next, 180);
    else setBox(next);
  }

  function fit() {
    if (!app.layout || !app.svg) return;
    var size = canvasSize();
    animateBox(fitBox(layoutRect(app.layout), size.w, size.h, { pad: 24, maxScale: 1.15 }), 320);
  }

  // ---------------------------------------------------------- the canvas

  // showLayout replaces the drawing. Moving down a level zooms into the
  // part opened and grows the new view out of it; moving up shrinks the old
  // view and zooms out of the part it came from.
  function showLayout(previous, state, opts) {
    var canvas = byId("canvas");
    var layout = app.layout;
    var oldSvg = app.svg;
    var oldBox = app.box;
    var oldLayout = app.drawnLayout;
    app.drawnLayout = layout;
    Array.prototype.slice.call(canvas.querySelectorAll(".loading, .empty-state, .flow-index, .history-view")).forEach(function (el) {
      el.parentNode.removeChild(el);
    });
    var drawn = drawLayout(layout);
    app.nodeEls = drawn.nodeEls;
    app.edgeEls = drawn.edgeEls;
    app.svg = drawn.svg;
    app.content = drawn.content;
    if (drawn.svg) canvas.appendChild(drawn.svg);
    if (drawn.overlay) canvas.appendChild(drawn.overlay);
    var target = drawn.svg ? homeBox(layout) : null;
    if (drawn.svg && layout.kind === "sequence") target = stepBox(target, state.step) || target;
    refreshCanvasState();
    var animate = !reduceMotion && oldSvg && drawn.svg && !opts.instant;
    if (!animate) {
      if (oldSvg && oldSvg.parentNode) oldSvg.parentNode.removeChild(oldSvg);
      if (target) {
        app.box = null;
        setBox(target);
      }
      return;
    }
    var size = canvasSize();
    var depthNew = DEPTH[state.view] || 0;
    var depthOld = DEPTH[previous.view] || 0;
    var oldEnd = scaleBox(oldBox, 1.04);
    var newStart = scaleBox(target, 0.97);
    if (depthNew > depthOld) {
      var into = rectOf(oldLayout, state.of) || rectOf(oldLayout, opts.anchor);
      if (into) oldEnd = fitBox(into, size.w, size.h, { pad: 8, maxScale: 6 });
      newStart = scaleBox(target, 1.7);
    } else if (depthNew < depthOld) {
      var from = rectOf(layout, previous.of) || rectOf(layout, previous.node);
      oldEnd = scaleBox(oldBox, 1.7);
      if (from) newStart = fitBox(from, size.w, size.h, { pad: 8, maxScale: 6 });
    }
    oldSvg.classList.add("leaving");
    var ms = 420;
    var start = null;
    if (app.animating) cancelAnimationFrame(app.animating);
    app.box = newStart;
    function frame(now) {
      if (start === null) start = now;
      var t = Math.min((now - start) / ms, 1);
      var e = ease(t);
      var ob = lerpBox(oldBox, oldEnd, e);
      oldSvg.setAttribute("viewBox", [ob.x, ob.y, ob.width, ob.height].join(" "));
      oldSvg.style.opacity = String(Math.max(0, 1 - t * 1.8));
      setBox(lerpBox(newStart, target, e));
      app.svg.style.opacity = String(Math.min(1, Math.max(0, (t - 0.2) / 0.6)));
      if (t < 1) {
        app.animating = requestAnimationFrame(frame);
      } else {
        app.animating = null;
        app.svg.style.opacity = "";
        if (oldSvg.parentNode) oldSvg.parentNode.removeChild(oldSvg);
      }
    }
    app.svg.style.opacity = "0";
    app.animating = requestAnimationFrame(frame);
  }

  function defs(root) {
    var d = svg("defs", {}, root);
    function arrow(id, cls, open) {
      var m = svg("marker", { id: id, viewBox: "0 0 10 10", refX: 9.5, refY: 5, markerWidth: 8, markerHeight: 8, orient: "auto-start-reverse", markerUnits: "userSpaceOnUse" }, d);
      if (open) svg("path", { d: "M1,1 L9.5,5 L1,9", class: "arrowhead open " + cls }, m);
      else svg("path", { d: "M0,0.5 L10,5 L0,9.5 z", class: "arrowhead " + cls }, m);
    }
    arrow("arrow", "");
    arrow("arrow-lit", "lit");
    arrow("arrow-faded", "faded");
    arrow("arrow-added", "added");
    arrow("arrow-changed", "changed");
    arrow("arrow-removed", "removed");
    arrow("arrow-open", "", true);
    arrow("arrow-open-lit", "lit", true);
    var many = svg("marker", { id: "er-many", viewBox: "0 0 14 14", refX: 0.5, refY: 7, markerWidth: 14, markerHeight: 14, orient: "auto-start-reverse", markerUnits: "userSpaceOnUse" }, d);
    svg("path", { d: "M13,7 L0.5,7 M13,7 L1,1.5 M13,7 L1,12.5 M13,2 L13,12", class: "er-mark" }, many);
    var one = svg("marker", { id: "er-one", viewBox: "0 0 14 14", refX: 13.5, refY: 7, markerWidth: 14, markerHeight: 14, orient: "auto", markerUnits: "userSpaceOnUse" }, d);
    svg("path", { d: "M0,7 L13.5,7 M9,2 L9,12 M12.5,2 L12.5,12", class: "er-mark" }, one);
  }

  function markerFor(edge, lit, open) {
    if (lit) return open ? "url(#arrow-open-lit)" : "url(#arrow-lit)";
    if (open) return "url(#arrow-open)";
    if (edge.change) return "url(#arrow-" + edge.change + ")";
    if (edge.faded) return "url(#arrow-faded)";
    return "url(#arrow)";
  }

  // drawLayout draws one view: an SVG for diagrams, HTML for the flows
  // index, and an empty-state card when a view has nothing yet.
  function drawLayout(layout) {
    var result = { svg: null, content: null, overlay: null, nodeEls: dict(), edgeEls: [] };
    if (layout.kind === "flows") {
      result.overlay = drawFlowIndex(layout);
      return result;
    }
    if (layout.kind === "history") {
      result.overlay = drawHistory(layout);
      return result;
    }
    if (layout.empty) {
      result.overlay = emptyCard(layout.empty);
      return result;
    }
    var root = svg("svg", {
      class: "diagram kind-" + layout.kind,
      role: "group",
      "aria-label": viewLabel(layout),
      preserveAspectRatio: "xMidYMid meet",
    });
    defs(root);
    var content = svg("g", { class: "content" }, root);
    result.svg = root;
    result.content = content;
    switch (layout.kind) {
      case "overview":
        drawOverview(layout, content, result);
        break;
      case "sequence":
        drawSequence(layout, content, result);
        break;
      case "er":
        drawEr(layout, content, result);
        break;
      default:
        drawGraph(layout, content, result);
    }
    wireCanvas(root);
    return result;
  }

  function viewLabel(layout) {
    var names = {
      overview: "Component group overview",
      context: "System context",
      containers: "Containers",
      inside: "Inside " + (layout.title || ""),
      group: (layout.title || "") + " group",
      focus: "Focus on " + (layout.title || ""),
      sequence: "Sequence diagram: " + (layout.title || ""),
      graph: "Diagram: " + (layout.title || ""),
      state: "State diagram: " + (layout.title || ""),
      er: "Data diagram: " + (layout.title || ""),
    };
    return names[layout.kind] || "Diagram";
  }

  function emptyCard(empty) {
    var card = html("div", { class: "empty-state", role: "status" });
    var inner = html("div", { class: "empty-inner" }, card);
    htmlIcon("diagram", inner, "empty-icon");
    html("h2", {}, inner, empty.title);
    html("p", {}, inner, empty.text);
    return card;
  }

  function nodeClasses(n, base) {
    var classes = [base || "node"];
    if (n.kind) classes.push("kind-" + n.kind);
    if (n.group) classes.push("group-" + n.group);
    if (n.shape) classes.push("shape-" + n.shape);
    var style = changeStyle(n.change);
    if (style) classes.push(style.className);
    if (n.faded) classes.push("faded");
    if (n.boundary) classes.push("boundary");
    if (n.command) classes.push("command");
    if (n.testSupport) classes.push("test-support");
    if (n.focus) classes.push("focus-node");
    if (n.stub) classes.push("stub");
    if (!n.ref) classes.push("inert");
    return classes.join(" ");
  }

  var KIND_ICON = {
    person: "person",
    system: "system",
    "external-system": "external-system",
    process: "process",
    "user-interface": "user-interface",
    cli: "cli",
    store: "store",
  };

  function tooltip(n) {
    var node = n.ref && app.data.nodeById[n.ref];
    var text = accessibleName(n);
    if (node && node.summary && node.summary.text) text += "\n" + node.summary.text;
    return text;
  }

  function badge(g, n) {
    var style = changeStyle(n.change);
    if (style && style.badge) {
      var b = svg("g", { class: "change-badge", transform: "translate(" + (n.width - 4) + ",4)" }, g);
      svg("circle", { r: 9 }, b);
      svg("text", { "text-anchor": "middle", y: 4 }, b).textContent = style.badge;
    }
  }

  // drawBox draws a node of the graph-like views: a card with an icon, its
  // title, and a subtitle, shaped by what it is.
  function drawBox(parent, n, result) {
    var g = svg("g", {
      class: nodeClasses(n),
      transform: "translate(" + n.x + "," + n.y + ")",
      "data-id": n.id,
      tabindex: n.ref ? 0 : -1,
      role: n.ref ? "button" : "img",
      "aria-label": accessibleName(n),
    }, parent);
    svg("title", {}, g).textContent = tooltip(n);
    var w = n.width;
    var h = n.height;
    if (n.shape === "initial") {
      svg("circle", { cx: w / 2, cy: h / 2, r: w / 2, class: "initial-dot" }, g);
      result.nodeEls[n.id] = g;
      return g;
    }
    if (n.kind === "store") {
      var e = 7;
      svg("path", {
        class: "body",
        d: "M0," + e + " A" + w / 2 + "," + e + " 0 0 1 " + w + "," + e + " V" + (h - e) + " A" + w / 2 + "," + e + " 0 0 1 0," + (h - e) + " Z",
      }, g);
      svg("path", { class: "rim", d: "M0," + e + " A" + w / 2 + "," + e + " 0 0 0 " + w + "," + e }, g);
    } else {
      svg("rect", { class: "body", width: w, height: h, rx: n.shape === "state" ? Math.min(h / 2, 18) : n.boundary ? 8 : 10 }, g);
    }
    if ((!n.shape || n.shape === "part") && !n.boundary && n.kind !== "system" && !n.focus) {
      svg("rect", { class: "accent", x: 0, y: 10, width: 3, height: h - 20, rx: 1.5 }, g);
    }
    var iconName = n.shape ? null : own(KIND_ICON, n.kind) || (n.kind === "package" && !n.boundary ? "package" : null);
    var tx = 14;
    if (iconName && !n.focus && !list(n.summaryLines).length && h >= 40) {
      svgIcon(iconName, g, 13, h / 2 - 9, 18);
      tx = 40;
    }
    var anchor = n.shape === "state" ? "middle" : "start";
    if (anchor === "middle") tx = w / 2;
    if (n.focus || list(n.summaryLines).length) {
      svgIcon(own(KIND_ICON, n.kind) || "package", g, 16, 16, 20);
      svg("text", { class: "title focus-title", x: 46, y: 32 }, g).textContent = n.title;
      svg("text", { class: "subtitle", x: 46, y: 50 }, g).textContent = n.subtitle || "";
      svg("line", { class: "divider", x1: 16, x2: w - 16, y1: 66, y2: 66 }, g);
      list(n.summaryLines).forEach(function (line, i) {
        svg("text", { class: "summary", x: 18, y: 88 + i * 17 }, g).textContent = line;
      });
    } else if (list(n.lines).length > 1) {
      var top = h / 2 - (n.lines.length * 16) / 2 + 12;
      n.lines.forEach(function (line, k) {
        svg("text", { class: "title", x: tx, y: top + k * 16, "text-anchor": anchor }, g).textContent = line;
      });
    } else if (n.subtitle && h >= 36) {
      svg("text", { class: "title", x: tx, y: h / 2 - 2, "text-anchor": anchor }, g).textContent = n.title;
      svg("text", { class: "subtitle", x: tx, y: h / 2 + 14, "text-anchor": anchor }, g).textContent = n.subtitle;
    } else {
      svg("text", { class: "title", x: tx, y: h / 2 + 4.5, "text-anchor": anchor }, g).textContent = n.title;
    }
    if (n.change === "removed") {
      svg("line", { class: "strike", x1: 8, x2: w - 8, y1: h / 2, y2: h / 2 }, g);
    }
    badge(g, n);
    result.nodeEls[n.id] = g;
    return g;
  }

  function drawEdge(parent, e, i, result, opts) {
    opts = opts || {};
    var classes = ["edge"];
    if (e.via) classes.push("via-" + e.via);
    var style = changeStyle(e.change);
    if (style) classes.push(style.className);
    if (e.faded) classes.push("faded");
    if (e.onDemand) classes.push("on-demand");
    var g = svg("g", { class: classes.join(" "), "data-index": i }, parent);
    var title = (titleOf(app.data, e.from) || e.from) + " → " + (titleOf(app.data, e.to) || e.to);
    if (e.label) title += ": " + e.label;
    if (e.via) title += " (" + e.via + ")";
    svg("title", {}, g).textContent = title;
    svg("path", { class: "hit", d: e.d }, g);
    var path = svg("path", { class: "line", d: e.d, "marker-end": opts.noArrow ? null : markerFor(e) }, g);
    if (opts.er) {
      path.setAttribute("marker-start", "url(#er-many)");
      path.setAttribute("marker-end", "url(#er-one)");
    }
    result.edgeEls[i] = g;
    return g;
  }

  function drawLabel(parent, e, i, dense) {
    var lines = list(e.labelLines);
    if (!lines.length) return;
    var g = svg("g", { class: "edge-label" + (e.onDemand || dense ? " on-demand" : ""), "data-index": i }, parent);
    var h = lines.length * LABEL_LINE + 6;
    svg("rect", { x: e.labelX - e.labelWidth / 2, y: e.labelY - h / 2, width: e.labelWidth, height: h, rx: 6 }, g);
    lines.forEach(function (line, k) {
      svg("text", { x: e.labelX, y: e.labelY - h / 2 + 3 + LABEL_LINE * (k + 1) - 3, "text-anchor": "middle" }, g).textContent = line;
    });
    return g;
  }

  function drawGraph(layout, content, result) {
    if (layout.boundary) {
      var b = layout.boundary;
      var bg = svg("g", { class: "boundary-box" }, content);
      svg("rect", { x: b.x, y: b.y, width: b.width, height: b.height, rx: 18 }, bg);
      svg("text", { class: "boundary-title", x: b.x + 18, y: b.y + 23 }, bg).textContent = b.title;
      svg("text", { class: "boundary-caption", x: b.x + 18 + textWidth(b.title, 14) + 12, y: b.y + 23 }, bg).textContent = "system boundary";
    }
    list(layout.lanes).forEach(function (lane, i) {
      var lg = svg("g", { class: "lane lane-" + (i % 2 ? "odd" : "even") + (layout.kind === "containers" ? " lane-column" : " lane-row") }, content);
      svg("rect", { x: lane.x, y: lane.y, width: lane.width, height: lane.height, rx: 12 }, lg);
      if (layout.kind === "containers") {
        svg("text", { class: "lane-title", x: lane.x + 16, y: lane.y + 26 }, lg).textContent = lane.title;
        svg("text", { class: "lane-count", x: lane.x + lane.width - 16, y: lane.y + 26, "text-anchor": "end" }, lg).textContent = String(lane.nodes.length);
      } else {
        laneTitleLines(lane.title).forEach(function (line, k) {
          svg("text", { class: "lane-title", x: lane.x + 16, y: lane.y + 26 + k * 15 }, lg).textContent = line;
        });
      }
    });
    list(layout.columns).forEach(function (col) {
      var cg = svg("g", { class: "column-head" }, content);
      svg("text", { x: col.x + col.width / 2, y: col.y + 12, "text-anchor": "middle" }, cg).textContent = col.title.toUpperCase() + "  ·  " + col.count;
      if (!col.count) {
        var focus = layout.nodes.filter(function (n) { return n.focus; })[0];
        svg("text", { class: "column-empty", x: col.x + col.width / 2, y: focus ? focus.y + focus.height / 2 : col.y + 60, "text-anchor": "middle" }, cg).textContent = col.id === "callers" ? "Nothing calls it" : "It depends on nothing";
      }
    });
    if (layout.kind === "group" || layout.kind === "inside") {
      var chips = layout.nodes.filter(function (n) { return n.boundary; });
      if (chips.length) {
        var top = Math.min.apply(null, chips.map(function (c) { return c.y; }));
        var left = Math.min.apply(null, chips.map(function (c) { return c.x; }));
        svg("text", { class: "chips-caption", x: left, y: top - 12 }, content).textContent = "ALSO DEPENDS ON, OUTSIDE THIS " + (layout.kind === "group" ? "GROUP" : "CONTAINER");
      }
    }
    var edgeLayer = svg("g", { class: "edges" }, content);
    layout.edges.forEach(function (e, i) {
      drawEdge(edgeLayer, e, i, result);
    });
    var nodeLayer = svg("g", { class: "nodes" }, content);
    layout.nodes.forEach(function (n) {
      drawBox(nodeLayer, n, result);
    });
    // In a busy view, labels show for the edges of the node in hand.
    var dense = layout.edges.length > DENSE_EDGES && layout.kind !== "state" && layout.kind !== "graph";
    if (dense) content.parentNode.classList.add("dense");
    var labelLayer = svg("g", { class: "labels" }, content);
    layout.edges.forEach(function (e, i) {
      drawLabel(labelLayer, e, i, dense);
    });
  }

  var DENSE_EDGES = 18;

  function drawOverview(layout, content, result) {
    layout.panels.forEach(function (p) {
      var g = svg("g", {
        class: "group-panel group-" + p.id,
        "data-panel": p.id,
        tabindex: 0,
        role: "button",
        "aria-label": "Open the " + p.label + " group, " + p.count + " packages",
      }, content);
      svg("rect", { class: "panel-body", x: p.x, y: p.y, width: p.width, height: p.height, rx: 14 }, g);
      var head = svg("g", { class: "panel-head" }, g);
      svg("rect", { class: "panel-head-hit", x: p.x, y: p.y, width: p.width, height: 42, rx: 14 }, head);
      svg("text", { x: p.x + OVERVIEW.pad, y: p.y + 28, class: "panel-title" }, head).textContent = p.label;
      svg("text", { x: p.x + p.width - OVERVIEW.pad, y: p.y + 28, class: "panel-count", "text-anchor": "end" }, head).textContent = p.count + (p.count === 1 ? " package  ›" : " packages  ›");
      p.subgroups.forEach(function (s) {
        svg("text", { x: s.x, y: s.y, class: "subgroup-label" }, g).textContent = s.label.toUpperCase();
      });
    });
    var arrows = svg("g", { class: "edges" }, content);
    layout.edges.forEach(function (e, i) {
      var g = svg("g", { class: "edge group-arrow", "data-index": i }, arrows);
      svg("title", {}, g).textContent = groupLabel(e.from) + " imports " + groupLabel(e.to) + ": " + e.count + (e.count === 1 ? " import edge" : " import edges");
      svg("line", { class: "line", x1: e.x1, y1: e.y1, x2: e.x2, y2: e.y2, "marker-end": "url(#arrow)" }, g);
      var text = String(e.count);
      var w = 14 + 7 * text.length;
      svg("rect", { x: e.labelX - w / 2, y: e.labelY - 10, width: w, height: 20, rx: 10, class: "count-pill" }, g);
      svg("text", { x: e.labelX, y: e.labelY + 4, "text-anchor": "middle", class: "count" }, g).textContent = text;
      result.edgeEls[i] = g;
    });
    var nodes = svg("g", { class: "nodes" }, content);
    layout.nodes.forEach(function (n) {
      var path = packagePath(n.id);
      var pkg = path ? app.data.packageByPath[path] : null;
      var chip = {
        id: n.id,
        ref: n.id,
        title: n.title,
        kind: "package",
        group: n.group,
        change: n.change,
        command: !!(pkg && pkg.command),
        testSupport: !!(pkg && pkg.testSupport),
        x: n.x,
        y: n.y,
        width: n.width,
        height: n.height,
      };
      var g = svg("g", {
        class: nodeClasses(chip, "node chip"),
        transform: "translate(" + n.x + "," + n.y + ")",
        "data-id": n.id,
        tabindex: 0,
        role: "button",
        "aria-label": accessibleName(chip),
      }, nodes);
      svg("title", {}, g).textContent = tooltip(chip);
      svg("rect", { class: "body", width: n.width, height: n.height, rx: 8 }, g);
      svg("circle", { class: "dot", cx: 14, cy: n.height / 2, r: 3.5 }, g);
      svg("text", { class: "title", x: 26, y: n.height / 2 + 4.5 }, g).textContent = n.title;
      if (chip.change === "removed") svg("line", { class: "strike", x1: 24, x2: n.width - 10, y1: n.height / 2, y2: n.height / 2 }, g);
      badge(g, chip);
      result.nodeEls[n.id] = g;
    });
  }

  function drawSequence(layout, content, result) {
    var width = layout.width;
    var bands = svg("g", { class: "step-bands" }, content);
    var lifelines = svg("g", { class: "lifelines" }, content);
    layout.participants.forEach(function (p) {
      svg("line", { class: "lifeline", x1: p.lifeline, x2: p.lifeline, y1: p.lifelineTop, y2: p.lifelineBottom }, lifelines);
    });
    var steps = svg("g", { class: "steps" }, content);
    app.stepEls = dict();
    layout.steps.forEach(function (s) {
      var top = s.box ? s.box.y - 8 : s.y - s.lines.length * 15 - 12;
      var bottom = s.box ? s.box.y + s.box.height + 8 : s.self ? s.y2 + 10 : s.y + 12;
      svg("rect", { class: "step-band", "data-step": s.number, x: 8, y: top, width: width - 16, height: bottom - top, rx: 8 }, bands);
      var cls = ["step", "kind-" + s.kind];
      var style = changeStyle(s.change);
      if (style) cls.push(style.className);
      var g = svg("g", { class: cls.join(" "), "data-step": s.number, tabindex: -1, role: "button", "aria-label": "Step " + s.number + ": " + s.label }, steps);
      svg("title", {}, g).textContent = "Step " + s.number + ": " + s.label + (s.via ? " (" + s.via + ")" : "");
      if (s.kind === "note") {
        svg("rect", { class: "note", x: s.box.x, y: s.box.y, width: s.box.width, height: s.box.height, rx: 6 }, g);
        s.lines.forEach(function (line, k) {
          svg("text", { class: "note-text", x: s.labelX, y: s.labelY + k * 15 + 2, "text-anchor": "middle" }, g).textContent = line;
        });
        var nb = svg("g", { class: "step-number", transform: "translate(" + (s.box.x + 2) + "," + (s.box.y + 2) + ")" }, g);
        svg("circle", { r: 10 }, nb);
        svg("text", { y: 4, "text-anchor": "middle" }, nb).textContent = String(s.number);
      } else {
        var open = s.kind === "return" || s.kind === "async";
        svg("path", { class: "hit", d: s.d }, g);
        svg("path", { class: "line", d: s.d, "marker-end": open ? "url(#arrow-open)" : "url(#arrow)" }, g);
        s.lines.forEach(function (line, k) {
          svg("text", { class: "step-label", x: s.labelX, y: s.labelY + k * 15, "text-anchor": s.anchor || "middle" }, g).textContent = line;
        });
        var dir = s.self ? 1 : s.x2 > s.x1 ? 1 : -1;
        var nx = s.x1 + dir * 15;
        var num = svg("g", { class: "step-number", transform: "translate(" + nx + "," + s.y + ")" }, g);
        svg("circle", { r: 10 }, num);
        svg("text", { y: 4, "text-anchor": "middle" }, num).textContent = String(s.number);
      }
      app.stepEls[s.number] = g;
    });
    var heads = svg("g", { class: "heads" }, content);
    svg("rect", { class: "heads-backdrop", x: -4000, y: layout.participants[0].y - 12, width: width + 8000, height: layout.participants[0].height + 22 }, heads);
    layout.participants.forEach(function (p) {
      drawBox(heads, p, result);
    });
    app.heads = heads;
    app.headsY = layout.participants[0].y;
    var footer = svg("g", { class: "feet" }, content);
    if (layout.steps.length > 6) {
      layout.participants.forEach(function (p) {
        var feet = copy(p);
        feet.y = p.lifelineBottom + 6;
        feet.height = 30;
        feet.subtitle = "";
        var g = drawBox(footer, feet, { nodeEls: dict() });
        g.setAttribute("tabindex", "-1");
        g.setAttribute("aria-hidden", "true");
        g.setAttribute("data-id", p.id);
        g.classList.add("foot");
      });
    }
  }

  // pinHeads keeps a flow's participant heads at the top of the canvas
  // while the view scrolls down its steps.
  function pinHeads() {
    if (!app.heads || !app.box || !app.layout || app.layout.kind !== "sequence" || !app.heads.isConnected) return;
    var shift = Math.max(0, app.box.y + 10 - app.headsY);
    app.heads.setAttribute("transform", "translate(0," + round(shift) + ")");
  }

  function drawEr(layout, content, result) {
    var edges = svg("g", { class: "edges" }, content);
    layout.edges.forEach(function (e, i) {
      drawEdge(edges, e, i, result, { er: true });
    });
    var nodes = svg("g", { class: "nodes" }, content);
    layout.nodes.forEach(function (n) {
      var g = svg("g", {
        class: nodeClasses(n, "node table-card"),
        transform: "translate(" + n.x + "," + n.y + ")",
        "data-id": n.id,
        tabindex: n.ref ? 0 : -1,
        role: n.ref ? "button" : "img",
        "aria-label": accessibleName(n),
      }, nodes);
      svg("title", {}, g).textContent = accessibleName(n);
      svg("rect", { class: "body", width: n.width, height: n.height, rx: 9 }, g);
      svg("path", { class: "table-head", d: "M0,9 A9,9 0 0 1 9,0 H" + (n.width - 9) + " A9,9 0 0 1 " + n.width + ",9 V" + (n.stub ? n.height - 9 : 34) + (n.stub ? " A9,9 0 0 1 " + (n.width - 9) + "," + n.height + " H9 A9,9 0 0 1 0," + (n.height - 9) : " H0") + " Z" }, g);
      svgIcon("table", g, 12, 9, 16);
      svg("text", { class: "title", x: 34, y: 22 }, g).textContent = n.title;
      svg("text", { class: "subtitle", x: n.width - 12, y: 22, "text-anchor": "end" }, g).textContent = n.stub ? "not in this diagram" : n.subtitle;
      if (n.missing) {
        svg("text", { class: "column-missing", x: 14, y: 52 }, g).textContent = "Not in the extracted facts yet";
      }
      n.columns.forEach(function (c, k) {
        var row = svg("g", { class: "column" + (c.primaryKey ? " pk" : "") + (c.foreignKey ? " fk" : "") }, g);
        if (k > 0) svg("line", { class: "row-rule", x1: 10, x2: n.width - 10, y1: c.y, y2: c.y }, row);
        var cy = c.y + c.height / 2 + 4;
        var kx = 12;
        if (c.primaryKey) {
          svg("text", { class: "key-tag pk-tag", x: kx, y: cy }, row).textContent = "PK";
        }
        if (c.foreignKey) {
          svg("text", { class: "key-tag fk-tag", x: c.primaryKey ? kx + 22 : kx, y: cy }, row).textContent = "FK";
        }
        var nameX = c.primaryKey && c.foreignKey ? 56 : 38;
        svg("text", { class: "column-name" + (c.notNull ? " not-null" : ""), x: nameX, y: cy }, row).textContent = c.name;
        svg("text", { class: "column-type" + (c.notNull ? "" : " nullable"), x: n.width - 12, y: cy, "text-anchor": "end" }, row).textContent = (c.type || "ANY") + (c.notNull ? "" : "?");
      });
      result.nodeEls[n.id] = g;
    });
  }

  function drawFlowIndex(layout) {
    var wrap = html("div", { class: "flow-index" });
    if (layout.empty) {
      wrap.appendChild(emptyCard(layout.empty));
      return wrap;
    }
    html("h2", { class: "flow-index-title" }, wrap, "Key runtime flows");
    html("p", { class: "flow-index-lead" }, wrap, "Each flow is a sequence diagram. Open one and step through it with the arrow keys.");
    var grid = html("ul", { class: "flow-grid" }, wrap);
    layout.items.forEach(function (f) {
      var li = html("li", {}, grid);
      var style = changeStyle(f.change);
      var card = html("button", { type: "button", class: "flow-card" + (style ? " " + style.className : "") }, li);
      var head = html("span", { class: "flow-card-head" }, card);
      htmlIcon("flow", head);
      html("span", { class: "flow-card-title" }, head, f.title);
      if (style) html("span", { class: "change-pill " + style.className }, head, style.label);
      if (f.summary && f.summary.text) html("span", { class: "flow-card-summary" }, card, f.summary.text);
      if (f.trigger && f.trigger.text) html("span", { class: "flow-card-trigger" }, card, "Starts when: " + f.trigger.text);
      html("span", { class: "flow-card-meta" }, card, f.participants + " participants · " + f.steps + " steps");
      card.addEventListener("click", function () {
        go({ view: "flow", of: f.id, step: 1 });
      });
    });
    return wrap;
  }

  // drawHistory draws the History timeline: a section per month, newest
  // first, and a card per entry.
  function drawHistory(layout) {
    var wrap = html("div", { class: "history-view" });
    app.shownEntry = null; // a fresh timeline brings the selected entry into view
    if (layout.empty) {
      wrap.appendChild(emptyCard(layout.empty));
      return wrap;
    }
    var inner = html("div", { class: "history-inner" }, wrap);
    var count = 0;
    layout.months.forEach(function (m) {
      count += m.entries.length;
    });
    html("h2", { class: "history-title" }, inner, "History");
    html("p", { class: "history-lead" }, inner, "Every merge to master, newest first, from CHANGELOG.md · " + count + (count === 1 ? " entry" : " entries"));
    layout.months.forEach(function (m) {
      var section = html("section", { class: "history-month", "aria-label": m.label }, inner);
      var h = html("h3", { class: "history-month-title" }, section);
      html("span", {}, h, m.label);
      html("span", { class: "history-count" }, h, String(m.entries.length));
      var ol = html("ol", { class: "history-list" }, section);
      m.entries.forEach(function (e) {
        drawHistoryCard(e, html("li", { class: "history-entry", "data-entry": e.id }, ol));
      });
    });
    return wrap;
  }

  function drawHistoryCard(e, li) {
    html("span", { class: "history-dot", "aria-hidden": "true" }, li);
    var card = html("article", { class: "history-card", "aria-label": e.title }, li);
    var head = html("header", { class: "history-card-head" }, card);
    html("time", { class: "history-date", datetime: e.date }, head, e.dateLabel);
    var title = html("h4", { class: "history-card-title" }, head);
    var pick = html("button", { type: "button", class: "history-pick", title: "Link to this entry" }, title, e.title);
    pick.addEventListener("click", function () {
      go({ view: "history", entry: e.id }, { replace: true });
    });
    var meta = html("p", { class: "history-meta" }, card);
    var branch = html("span", { class: "history-branch", title: "Branch" }, meta);
    htmlIcon("branch", branch);
    html("code", {}, branch, e.branch);
    if (e.merge) {
      var merge = html("span", { class: "history-commit" }, meta);
      htmlIcon("commit", merge);
      html("span", {}, merge, "Merge ");
      commitAnchor(e.merge, merge);
    } else if (e.commits) {
      var range = html("span", { class: "history-commit" }, meta);
      htmlIcon("commit", range);
      html("span", {}, range, "Commits ");
      commitAnchor(e.commits.from, range);
      html("span", { class: "history-range-dots", "aria-label": "to" }, range, "…");
      commitAnchor(e.commits.to, range);
    } else {
      html("span", { class: "history-pending" }, meta, "No merge commit yet");
    }
    var text = html("div", { class: "history-text" }, card);
    e.text.forEach(function (paragraph) {
      renderInlineCode(paragraph, html("p", {}, text));
    });
    var foot = html("dl", { class: "history-facts" }, card);
    html("dt", {}, foot, "Touches");
    var touches = html("ul", { class: "touch-list" }, html("dd", {}, foot));
    e.touches.forEach(function (t) {
      var tli = html("li", {}, touches);
      var node = t.node ? app.data.nodeById[t.node] : null;
      if (node) {
        var b = html("button", { type: "button", class: "touch" + (node.group ? " group-" + node.group : ""), title: "Open " + (node.title || t.path) }, tli);
        html("span", { class: "dot", "aria-hidden": "true" }, b);
        html("code", {}, b, t.path);
        b.addEventListener("click", function () {
          jumpToNode(t.node);
        });
      } else {
        var gone = html("span", { class: "touch gone", title: "Not a package the explorer shows now" }, tli);
        html("code", {}, gone, t.path);
      }
    });
    if (e.sources.length) {
      html("dt", {}, foot, "Sources");
      var sources = html("ul", { class: "sources", "aria-label": "Sources" }, html("dd", {}, foot));
      e.sources.forEach(function (source) {
        renderRef(source, sources, card);
      });
    }
  }

  function commitAnchor(link, parent) {
    var el = link.href ? html("a", { href: link.href, target: "_blank", rel: "noopener", title: "Open on GitHub" }, parent) : html("span", {}, parent);
    html("code", {}, el, link.sha);
  }

  // renderInlineCode sets a paragraph's text, with spans between backticks
  // as code. Unbalanced backticks stay as written.
  function renderInlineCode(text, parent) {
    var parts = String(text).split("`");
    if (parts.length % 2 === 0) {
      parent.textContent = text;
      return;
    }
    parts.forEach(function (part, i) {
      if (i % 2) html("code", {}, parent, part);
      else if (part) parent.appendChild(document.createTextNode(part));
    });
  }

  // markHistoryEntry marks the entry the address selects and brings it into
  // view when the selection changes.
  function markHistoryEntry() {
    var view = byId("canvas").querySelector(".history-view");
    if (!view) return;
    var selected = null;
    Array.prototype.forEach.call(view.querySelectorAll(".history-entry"), function (li) {
      var on = li.getAttribute("data-entry") === app.state.entry;
      li.classList.toggle("selected", on);
      if (on) selected = li;
    });
    if (selected && app.shownEntry !== app.state.entry) {
      selected.scrollIntoView({ block: "center", behavior: reduceMotion ? "auto" : "smooth" });
    }
    app.shownEntry = app.state.entry;
  }

  // ------------------------------------------------------- canvas events

  function nodeTarget(event) {
    var el = event.target;
    while (el && el !== event.currentTarget) {
      if (el.getAttribute && el.getAttribute("data-id")) return el;
      if (el.getAttribute && el.getAttribute("data-panel")) return el;
      if (el.getAttribute && el.getAttribute("data-step") && el.classList.contains("step")) return el;
      el = el.parentNode;
    }
    return null;
  }

  function drawnNodeById(id) {
    var found = null;
    list(app.layout && app.layout.nodes).forEach(function (n) {
      if (!found && n.id === id) found = n;
    });
    return found;
  }

  function activate(el, open) {
    var panel = el.getAttribute("data-panel");
    if (panel) {
      go({ view: "group", of: panel }, { anchor: panel });
      return;
    }
    var step = el.getAttribute("data-step");
    if (step && !el.getAttribute("data-id")) {
      setStep(Number(step));
      return;
    }
    var n = drawnNodeById(el.getAttribute("data-id"));
    if (!n || !n.ref) return;
    if (open || app.state.node === n.ref) drill(n.ref, n.id);
    else select(n.ref);
  }

  // drill opens what a node stands for, one level down.
  function drill(ref, drawnId) {
    var node = app.data.nodeById[ref];
    if (!node) return;
    var anchor = drawnId || ref;
    if (node.kind === "system" && app.state.view === "context") {
      go({ view: "containers", node: ref }, { anchor: anchor });
    } else if (node.contains.length && !(app.state.view === "inside" && app.state.of === ref)) {
      go({ view: "inside", of: ref }, { anchor: anchor });
    } else if (app.state.view === "focus" && app.state.of === ref) {
      if (node.diagrams.length) go({ view: "diagram", of: node.diagrams[0].id, node: ref }, { anchor: anchor });
    } else {
      go({ view: "focus", of: ref, node: ref }, { anchor: anchor });
    }
  }

  function wireCanvas(root) {
    var drag = null;
    root.addEventListener("click", function (event) {
      if (app.dragged) {
        app.dragged = false;
        return;
      }
      var el = nodeTarget(event);
      if (el) activate(el, false);
      else if (app.state.node || app.state.term) select("");
    });
    root.addEventListener("dblclick", function (event) {
      var el = nodeTarget(event);
      if (el) {
        event.preventDefault();
        activate(el, true);
      }
    });
    root.addEventListener("keydown", function (event) {
      if (event.key !== "Enter" && event.key !== " ") return;
      var el = nodeTarget(event);
      if (!el) return;
      event.preventDefault();
      activate(el, event.shiftKey);
    });
    root.addEventListener("mouseover", function (event) {
      var el = nodeTarget(event);
      var id = el && el.getAttribute("data-id");
      if (id && id !== app.hover) light(id);
    });
    root.addEventListener("mouseout", function (event) {
      var to = event.relatedTarget;
      var el = nodeTarget(event);
      if (!el) return;
      while (to && to !== root) {
        if (to === el) return;
        to = to.parentNode;
      }
      light(null);
    });
    root.addEventListener("focusin", function (event) {
      var el = nodeTarget(event);
      var id = el && el.getAttribute("data-id");
      if (id) light(id);
    });
    root.addEventListener("focusout", function () {
      light(null);
    });
    root.addEventListener("pointerdown", function (event) {
      if (event.button !== 0) return;
      drag = { x: event.clientX, y: event.clientY, box: app.box, moved: false, id: event.pointerId };
    });
    root.addEventListener("pointermove", function (event) {
      if (!drag || drag.id !== event.pointerId) return;
      var dx = event.clientX - drag.x;
      var dy = event.clientY - drag.y;
      if (!drag.moved && Math.hypot(dx, dy) < 4) return;
      if (!drag.moved) {
        drag.moved = true;
        root.setPointerCapture(event.pointerId);
        root.classList.add("panning");
      }
      var rect = root.getBoundingClientRect();
      var k = drag.box.width / rect.width;
      setBox({ x: drag.box.x - dx * k, y: drag.box.y - dy * k, width: drag.box.width, height: drag.box.height });
    });
    function endDrag(event) {
      if (!drag || drag.id !== event.pointerId) return;
      if (drag.moved) {
        app.dragged = true;
        setTimeout(function () {
          app.dragged = false;
        }, 0);
      }
      root.classList.remove("panning");
      drag = null;
    }
    root.addEventListener("pointerup", endDrag);
    root.addEventListener("pointercancel", endDrag);
    root.addEventListener(
      "wheel",
      function (event) {
        event.preventDefault();
        var rect = root.getBoundingClientRect();
        var fx = app.box.x + ((event.clientX - rect.left) / rect.width) * app.box.width;
        var fy = app.box.y + ((event.clientY - rect.top) / rect.height) * app.box.height;
        var speed = event.ctrlKey ? 0.01 : 0.0018;
        zoomBy(Math.exp(-event.deltaY * speed), fx, fy);
      },
      { passive: false }
    );
  }

  // light highlights a node, its neighbours, and the edges between them,
  // and dims the rest.
  function light(id) {
    app.hover = id;
    if (!app.svg || !app.layout) return;
    var lit = id ? neighbours(app.layout, id) : { nodes: [], edges: [] };
    app.svg.classList.toggle("dimming", !!id && lit.nodes.length > 0);
    Object.keys(app.nodeEls).forEach(function (key) {
      app.nodeEls[key].classList.toggle("lit", lit.nodes.indexOf(key) >= 0);
    });
    app.edgeEls.forEach(function (el, i) {
      if (!el) return;
      var on = lit.edges.indexOf(i) >= 0;
      el.classList.toggle("lit", on);
      var line = el.querySelector(".line");
      var e = app.layout.edges[i];
      if (line && e && e.d && line.getAttribute("marker-end")) line.setAttribute("marker-end", markerFor(e, on));
    });
    Array.prototype.forEach.call(app.svg.querySelectorAll(".edge-label"), function (el) {
      el.classList.toggle("lit", lit.edges.indexOf(Number(el.getAttribute("data-index"))) >= 0);
    });
  }

  // refreshCanvasState marks the selected node and the current flow step
  // without redrawing.
  function refreshCanvasState() {
    if (app.layout && app.layout.kind === "history") markHistoryEntry();
    if (!app.svg) return;
    var selected = app.state.node;
    Object.keys(app.nodeEls).forEach(function (key) {
      var n = drawnNodeById(key);
      app.nodeEls[key].classList.toggle("selected", !!(n && n.ref && n.ref === selected));
    });
    var sel = selected ? neighbours(app.layout, drawnIdFor(selected)) : { nodes: [], edges: [] };
    app.edgeEls.forEach(function (el, i) {
      if (el) el.classList.toggle("shown", sel.edges.indexOf(i) >= 0);
    });
    Array.prototype.forEach.call(app.svg.querySelectorAll(".edge-label.on-demand"), function (el) {
      el.classList.toggle("shown", sel.edges.indexOf(Number(el.getAttribute("data-index"))) >= 0);
    });
    if (app.layout.kind === "sequence") markStep();
  }

  function drawnIdFor(ref) {
    var found = null;
    list(app.layout && app.layout.nodes).forEach(function (n) {
      if (!found && n.ref === ref) found = n.id;
    });
    return found;
  }

  // ------------------------------------------------------------- stepping

  function stepCount() {
    return app.layout && app.layout.kind === "sequence" ? app.layout.steps.length : 0;
  }

  function markStep() {
    var current = app.state.step || 0;
    var step = null;
    app.layout.steps.forEach(function (s) {
      var el = app.stepEls && app.stepEls[s.number];
      if (!el) return;
      el.classList.toggle("current", s.number === current);
      el.classList.toggle("later", current > 0 && s.number > current);
      if (s.number === current) step = s;
    });
    Array.prototype.forEach.call(app.svg.querySelectorAll(".step-band"), function (el) {
      el.classList.toggle("current", Number(el.getAttribute("data-step")) === current);
    });
    Object.keys(app.nodeEls).forEach(function (key) {
      app.nodeEls[key].classList.toggle("in-step", !!step && (step.from === key || step.to === key));
    });
  }

  function stepBox(box, number) {
    if (!app.layout || app.layout.kind !== "sequence" || !number || !box) return null;
    var s = app.layout.steps[number - 1];
    if (!s) return null;
    var lo = s.box ? s.box.x : Math.min(s.x1, s.x2) - 30;
    var hi = s.box ? s.box.x + s.box.width : Math.max(s.x1, s.x2) + (s.self ? 200 : 30);
    if (hi - lo > box.width - 40 && !s.box) {
      // Too wide to show whole: show the sender and the label beside it.
      lo = Math.min(s.x1, s.labelX - s.labelWidth / 2) - 60;
      hi = Math.max(s.x1, s.labelX + s.labelWidth / 2) + 60;
    }
    var y = s.box ? s.box.y : s.y - s.lines.length * 15;
    var headroom = app.layout.participants[0].height + 40;
    var next = { x: box.x, y: box.y, width: box.width, height: box.height };
    if (lo < next.x + 20 || hi > next.x + next.width - 20) {
      next.x = (lo + hi) / 2 - next.width / 2;
      next.x = Math.min(Math.max(next.x, -40), Math.max(app.layout.width - next.width + 40, -40));
    }
    if (y < next.y + headroom || y > next.y + next.height * 0.75) {
      next.y = Math.max(-8, y - next.height * 0.38);
    }
    return next;
  }

  function setStep(n) {
    var count = stepCount();
    if (!count) return;
    var step = Math.min(Math.max(n, 1), count);
    var target = stepBox(app.box, step);
    select(app.state.node, { step: step });
    if (target && (target.x !== app.box.x || target.y !== app.box.y)) animateBox(target, 360);
  }

  function renderStepper() {
    var el = byId("stepper");
    var count = stepCount();
    clear(el);
    el.hidden = !count;
    if (!count) return;
    var current = app.state.step || 1;
    var prev = html("button", { type: "button", class: "tool", "aria-label": "Previous step", title: "Previous step (←)" }, el);
    htmlIcon("prev", prev);
    prev.disabled = current <= 1;
    prev.addEventListener("click", function () {
      setStep(current - 1);
    });
    html("span", { class: "step-count", "aria-live": "polite" }, el, "Step " + current + " of " + count);
    var next = html("button", { type: "button", class: "tool", "aria-label": "Next step", title: "Next step (→)" }, el);
    htmlIcon("next", next);
    next.disabled = current >= count;
    next.addEventListener("click", function () {
      setStep(current + 1);
    });
  }

  // --------------------------------------------------------------- chrome

  function renderLevels() {
    var nav = byId("levels");
    clear(nav);
    var level = levelOf(app.state.view);
    if (app.state.view === "focus" && app.data.nodeById[app.state.of]) {
      var focused = app.data.nodeById[app.state.of];
      if (focused.level === "context") level = "context";
      if (focused.level === "container") level = "containers";
    }
    LEVELS.forEach(function (l) {
      var b = html("button", { type: "button", class: "level" + (l.id === level ? " active" : ""), "aria-current": l.id === level ? "page" : null }, nav, l.label);
      b.addEventListener("click", function () {
        go({ view: l.id === "components" ? "components" : l.id });
      });
    });
  }

  function renderCrumbs() {
    var nav = byId("crumbs");
    clear(nav);
    var ol = html("ol", {}, nav);
    breadcrumb(app.data, app.state).forEach(function (c, i, all) {
      var li = html("li", {}, ol);
      if (c.state && i < all.length - 1) {
        var b = html("button", { type: "button", class: "crumb" }, li, c.label);
        b.addEventListener("click", function () {
          go(c.state);
        });
      } else {
        html("span", { class: "crumb current", "aria-current": "page" }, li, c.label);
      }
    });
  }

  function renderNotice(text) {
    var el = byId("notice");
    el.hidden = !text;
    el.textContent = text || "";
  }

  function renderProblems() {
    var banner = byId("problems");
    banner.hidden = app.problems.length === 0;
    clear(banner);
    app.problems.forEach(function (p) {
      html("p", {}, banner, p);
    });
  }

  var LEGENDS = {
    overview: [
      ["chip", "Package"],
      ["chip command", "Command (bold)"],
      ["chip test-support", "Test support (dashed)"],
      ["count", "Imports between groups, with the count"],
    ],
    group: [
      ["chip", "Package in this group"],
      ["chip command", "Command (bold)"],
      ["chip boundary", "Outside this group (faded)"],
      ["arrow", "Imports; edges to outside packages show on hover"],
    ],
    inside: [
      ["chip", "Part of this container"],
      ["chip boundary", "Outside it (faded)"],
      ["arrow", "Imports or calls; edges outside show on hover"],
    ],
    focus: [
      ["chip", "Callers on the left, dependencies on the right"],
      ["arrow", "Imports or calls, with how"],
    ],
    context: [
      ["icon person", "Person"],
      ["icon system", "Evie"],
      ["icon external-system", "Outside system"],
      ["arrow", "Calls, labelled with what for"],
      ["via http", "Over HTTP or the network"],
      ["via process", "Runs or is run as a process"],
      ["via sql", "Reads or writes data or files"],
    ],
    containers: [
      ["icon user-interface", "User interface"],
      ["icon cli", "Command-line tool"],
      ["icon process", "Process"],
      ["icon store", "Data store"],
      ["icon external-system", "Outside service"],
      ["via http", "Over HTTP or the network"],
      ["via process", "Runs or is run as a process"],
      ["via sql", "SQL or files"],
    ],
    sequence: [
      ["arrow", "Call"],
      ["arrow open", "Return or async message (open head)"],
      ["note", "Note"],
      ["step", "Current step (← →)"],
    ],
    graph: [
      ["chip", "Part"],
      ["arrow", "Leads to"],
    ],
    state: [
      ["initial", "Initial state"],
      ["state", "State"],
      ["arrow", "Transition"],
    ],
    er: [
      ["pk", "PK primary key, FK foreign key"],
      ["nullable", "TYPE? may be null"],
      ["crow", "Many rows reference one"],
      ["chip boundary", "Referenced table not listed (faded)"],
    ],
    flows: [["flow", "Open a flow to step through it"]],
  };

  function legendSwatch(kind, li) {
    var parts = kind.split(" ");
    if (parts[0] === "icon") {
      htmlIcon(parts[1], li, "legend-icon kind-" + parts[1]);
      return;
    }
    if (parts[0] === "flow") {
      htmlIcon("flow", li, "legend-icon");
      return;
    }
    if (parts[0] === "via") {
      var line = svg("svg", { class: "legend-via via-" + parts[1], viewBox: "0 0 26 8", "aria-hidden": "true" }, li);
      svg("line", { x1: 0, y1: 4, x2: 26, y2: 4 }, line);
      return;
    }
    html("span", { class: "swatch-" + parts.join(" swatch-"), "aria-hidden": "true" }, li);
  }

  function renderLegend() {
    var ul = byId("legend");
    clear(ul);
    (LEGENDS[app.layout ? app.layout.kind : "context"] || []).forEach(function (entry) {
      var li = html("li", {}, ul);
      legendSwatch(entry[0], li);
      html("span", {}, li, entry[1]);
    });
    if (list(app.state.proposals).length) {
      proposalLegend().forEach(function (entry) {
        var li = html("li", { class: "proposal-legend" }, ul);
        html("span", { class: "swatch-change " + entry.className, "aria-hidden": "true" }, li);
        html("span", {}, li, entry.text);
      });
    }
  }

  function renderRail() {
    var groups = byId("group-list");
    clear(groups);
    groupOverview(app.data).groups.forEach(function (g) {
      var count = 0;
      g.subgroups.forEach(function (s) {
        count += s.nodes.length;
      });
      var li = html("li", {}, groups);
      var active = app.state.view === "group" && app.state.of === g.id;
      var b = html("button", { type: "button", class: "group-link group-" + g.id + (active ? " active" : ""), "aria-current": active ? "page" : null }, li);
      html("span", { class: "swatch", "aria-hidden": "true" }, b);
      html("span", { class: "group-name" }, b, g.label);
      html("span", { class: "rail-count" }, b, String(count));
      b.addEventListener("click", function () {
        go({ view: "group", of: g.id });
      });
    });
    renderLegend();
    var ul = byId("proposal-list");
    clear(ul);
    if (!app.base.proposals.length) {
      var li = html("li", { class: "hint" }, ul);
      html("span", {}, li, "No design proposals yet. ");
      html("a", { href: "?proposal=examples/design-proposal.js" + location.hash }, li, "Load the example");
      return;
    }
    app.base.proposals.forEach(function (p) {
      var active = list(app.state.proposals).indexOf(p.id) >= 0;
      var item = html("li", { class: "proposal" + (active ? " on" : "") }, ul);
      var label = html("label", { class: "switch" }, item);
      var box = html("input", { type: "checkbox", role: "switch", "aria-checked": active ? "true" : "false" }, label);
      box.checked = active;
      html("span", { class: "switch-track", "aria-hidden": "true" }, label);
      var text = html("span", { class: "proposal-text" }, label);
      html("span", { class: "proposal-title" }, text, p.title || p.id);
      html("span", { class: "status status-" + (p.status || "") }, text, p.status || "");
      box.addEventListener("change", function () {
        var ids = list(app.state.proposals).filter(function (id) {
          return id !== p.id;
        });
        if (box.checked) ids.push(p.id);
        var next = { view: app.state.view, of: app.state.of, step: app.state.step, proposals: ids };
        if (box.checked) next.panel = "proposals";
        go(next);
      });
      if (active) {
        var more = html("button", { type: "button", class: "linklike proposal-more" }, item, "Requirements and decisions");
        more.addEventListener("click", function () {
          go({ view: app.state.view, of: app.state.of, step: app.state.step, proposals: app.state.proposals, panel: "proposals" }, { replace: true });
        });
      }
    });
  }

  // --------------------------------------------------------------- search

  var searchState = { results: [], active: -1 };

  function renderSearch() {
    var input = byId("search-input");
    var ul = byId("search-results");
    clear(ul);
    var results = searchState.results;
    ul.hidden = !input.value.trim();
    input.setAttribute("aria-expanded", ul.hidden ? "false" : "true");
    if (!results.length) {
      html("li", { class: "search-empty", role: "option", "aria-disabled": "true" }, ul, "Nothing matches “" + input.value.trim() + "”");
      input.removeAttribute("aria-activedescendant");
      return;
    }
    var typeIcon = { node: "package", term: "term", flow: "flow", diagram: "diagram", history: "history" };
    var typeName = { node: "", term: "Glossary", flow: "Flow", diagram: "Diagram", history: "History" };
    results.forEach(function (r, i) {
      var li = html("li", { id: "search-result-" + i, role: "option", class: "search-result" + (i === searchState.active ? " active" : ""), "aria-selected": i === searchState.active ? "true" : "false" }, ul);
      var node = r.type === "node" ? app.data.nodeById[r.id] : null;
      htmlIcon(node ? own(KIND_ICON, node.kind) || "package" : typeIcon[r.type], li, "result-icon" + (node && node.group ? " group-" + node.group : ""));
      var text = html("span", { class: "result-text" }, li);
      html("span", { class: "result-label" }, text, r.label);
      html("span", { class: "result-detail" }, text, r.detail);
      if (typeName[r.type]) html("span", { class: "result-type" }, li, typeName[r.type]);
      li.addEventListener("mousedown", function (event) {
        event.preventDefault();
        choose(i);
      });
    });
    if (searchState.active >= 0) input.setAttribute("aria-activedescendant", "search-result-" + searchState.active);
    else input.removeAttribute("aria-activedescendant");
  }

  function choose(i) {
    var r = searchState.results[i];
    if (!r) return;
    var input = byId("search-input");
    input.value = "";
    searchState = { results: [], active: -1 };
    renderSearch();
    input.blur();
    if (r.type === "term") {
      go({ view: app.state.view, of: app.state.of, step: app.state.step, term: r.id, proposals: app.state.proposals }, { replace: true });
    } else {
      go(r.state);
    }
  }

  function wireSearch() {
    var input = byId("search-input");
    input.addEventListener("input", function () {
      searchState.results = search(app.data, input.value, 10);
      searchState.active = searchState.results.length ? 0 : -1;
      renderSearch();
    });
    input.addEventListener("keydown", function (event) {
      var n = searchState.results.length;
      if (event.key === "ArrowDown" && n) {
        event.preventDefault();
        searchState.active = (searchState.active + 1) % n;
        renderSearch();
      } else if (event.key === "ArrowUp" && n) {
        event.preventDefault();
        searchState.active = (searchState.active - 1 + n) % n;
        renderSearch();
      } else if (event.key === "Enter") {
        event.preventDefault();
        choose(Math.max(searchState.active, 0));
      } else if (event.key === "Escape") {
        input.value = "";
        searchState = { results: [], active: -1 };
        renderSearch();
        input.blur();
      }
    });
    input.addEventListener("blur", function () {
      setTimeout(function () {
        byId("search-results").hidden = true;
        input.setAttribute("aria-expanded", "false");
      }, 120);
    });
    input.addEventListener("focus", function () {
      if (input.value.trim()) renderSearch();
    });
  }

  // ---------------------------------------------------------------- panel

  // jump follows an in-page reference: a node it draws is selected where it
  // stands, anything else opens where it lives.
  function jumpToNode(id, flash) {
    app.flash = flash || null;
    var node = app.data.nodeById[id];
    if (!node) return;
    if (drawnIdFor(id)) {
      select(id);
      var n = drawnNodeById(drawnIdFor(id));
      var size = canvasSize();
      if (n && app.box && (n.x < app.box.x || n.y < app.box.y || n.x + n.width > app.box.x + app.box.width || n.y + n.height > app.box.y + app.box.height)) {
        animateBox(fitBox({ x: n.x - 200, y: n.y - 150, width: n.width + 400, height: n.height + 300 }, size.w, size.h, { maxScale: 1 }), 360);
      }
      return;
    }
    go(nodeHome(node));
  }

  function renderRef(ref, parent, statementEl) {
    var link = resolveSource(ref, app.data, app.links);
    var li = html("li", { class: "source source-" + link.kind }, parent);
    var iconName = { code: "code", doc: "doc", adr: "adr", term: "term", pkg: "package", node: "node", table: "table", route: "route", command: "command", issue: "issue" }[link.kind] || "doc";
    var el;
    if (link.href) {
      el = html("a", { href: link.href, target: "_blank", rel: "noopener" }, li);
    } else if (link.kind === "term") {
      el = html("button", { type: "button", class: "ref-button", "aria-expanded": "false" }, li);
      el.addEventListener("click", function () {
        var open = el.getAttribute("aria-expanded") === "true";
        el.setAttribute("aria-expanded", open ? "false" : "true");
        var existing = statementEl.querySelector('.term-def[data-term="' + cssEscape(link.text) + '"]');
        if (existing) existing.parentNode.removeChild(existing);
        if (!open) {
          var def = html("div", { class: "term-def", "data-term": link.text }, statementEl);
          html("strong", {}, def, link.text);
          html("span", {}, def, " " + (link.definition || "is not in GLOSSARY.md."));
          var more = html("button", { type: "button", class: "linklike" }, def, "Who uses it");
          more.addEventListener("click", function () {
            select(app.state.node, { term: link.text, node: undefined });
          });
        }
      });
    } else if (link.target && link.target.node) {
      el = html("button", { type: "button", class: "ref-button" }, li);
      el.addEventListener("click", function () {
        jumpToNode(link.target.node, link.target.type === "node" ? null : link.target);
      });
    } else if (link.target && link.target.type === "node" && app.data.nodeById[link.target.value]) {
      el = html("button", { type: "button", class: "ref-button" }, li);
      el.addEventListener("click", function () {
        jumpToNode(link.target.value);
      });
    } else {
      el = html("span", { class: "ref-text" }, li);
    }
    htmlIcon(iconName, el, "ref-icon");
    html("span", { class: "ref-label" }, el, link.text);
    if (link.title) el.setAttribute("title", link.title);
  }

  function cssEscape(text) {
    return String(text).replace(/["\\]/g, "\\$&");
  }

  function renderStatement(statement, parent, cls) {
    if (!statement) return null;
    var block = html("div", { class: "statement" + (cls ? " " + cls : "") }, parent);
    html("p", {}, block, statement.text || "");
    var refs = list(statement.refs);
    if (refs.length) {
      var ul = html("ul", { class: "sources", "aria-label": "Sources" }, block);
      refs.forEach(function (ref) {
        renderRef(ref, ul, block);
      });
    }
    return block;
  }

  function section(parent, title, count, key) {
    var s = html("section", { class: "panel-section" + (key ? " section-" + key : "") }, parent);
    var h = html("h3", {}, s, title);
    if (count !== undefined) html("span", { class: "count" }, h, String(count));
    return s;
  }

  var SHOW_FIRST = 6;

  function renderStatements(parent, key, title, statements) {
    if (!statements.length) return;
    var s = section(parent, title, statements.length > 1 ? statements.length : undefined, key);
    var expandKey = (app.state.node || "") + "|" + key;
    var all = app.expanded[expandKey] || statements.length <= SHOW_FIRST + 2;
    statements.slice(0, all ? statements.length : SHOW_FIRST).forEach(function (st) {
      renderStatement(st, s);
    });
    if (!all) {
      var more = html("button", { type: "button", class: "show-more" }, s, "Show all " + statements.length);
      more.addEventListener("click", function () {
        app.expanded[expandKey] = true;
        renderPanel();
      });
    }
  }

  function renderRelations(parent, title, items, empty, key) {
    var s = section(parent, title, items.length, key);
    if (!items.length) {
      html("p", { class: "empty" }, s, empty);
      return;
    }
    var ul = html("ul", { class: "relation-list" }, s);
    items.forEach(function (r) {
      var style = changeStyle(r.change);
      var li = html("li", { class: style ? style.className : null }, ul);
      var node = app.data.nodeById[r.id];
      var b = html("button", { type: "button", class: "relation" + (node && node.group ? " group-" + node.group : "") }, li);
      html("span", { class: "dot", "aria-hidden": "true" }, b);
      html("span", { class: "relation-title" }, b, r.title);
      if (r.label || (r.via && r.via !== "import")) html("span", { class: "relation-how" }, b, [r.label, r.via && r.via !== "import" ? r.via : ""].filter(Boolean).join(" · "));
      if (style) html("span", { class: "change-pill " + style.className }, b, style.label);
      b.addEventListener("click", function () {
        jumpToNode(r.id);
      });
    });
  }

  function panelHeader(panel, iconName, title, sub, onClose, cls) {
    var header = html("header", { class: "panel-header" + (cls ? " " + cls : "") }, panel);
    var badgeEl = html("span", { class: "panel-icon" }, header);
    htmlIcon(iconName, badgeEl);
    var titles = html("div", { class: "panel-titles" }, header);
    html("h2", {}, titles, title);
    if (sub) html("p", { class: "panel-sub" }, titles, sub);
    var close = html("button", { type: "button", class: "tool close", "aria-label": "Close details", title: "Close (Esc)" }, header);
    htmlIcon("close", close);
    close.addEventListener("click", onClose);
    return header;
  }

  function actionButton(parent, iconName, label, onClick, primary) {
    var b = html("button", { type: "button", class: "action" + (primary ? " primary" : "") }, parent);
    htmlIcon(iconName, b);
    html("span", {}, b, label);
    b.addEventListener("click", onClick);
    return b;
  }

  function renderPanel() {
    var panel = byId("panel");
    var scroll = panel.scrollTop;
    var sameSubject = panel.getAttribute("data-subject");
    clear(panel);
    var subject = "";
    if (app.state.node && app.data.nodeById[app.state.node]) {
      subject = "node:" + app.state.node;
      renderNodePanel(panel, app.state.node);
    } else if (app.state.term) {
      subject = "term:" + app.state.term;
      renderTermPanel(panel, app.state.term);
    } else if (app.state.panel === "proposals") {
      subject = "proposals";
      renderProposalPanel(panel);
    } else if (stepCount()) {
      subject = "flow:" + app.state.of;
      renderFlowPanel(panel);
    }
    panel.hidden = !subject;
    document.body.classList.toggle("has-panel", !!subject);
    panel.setAttribute("data-subject", subject);
    if (subject && subject === sameSubject) panel.scrollTop = scroll;
    else panel.scrollTop = 0;
    if (app.flash) {
      var hit = panel.querySelector(".flash");
      if (hit) hit.scrollIntoView({ block: "center" });
      app.flash = null;
    }
  }

  function closePanel() {
    go({ view: app.state.view, of: app.state.of, step: app.state.step, proposals: app.state.proposals }, { replace: true });
  }

  function renderNodePanel(panel, id) {
    var details = nodeDetails(app.data, id, app.links);
    var node = details.node;
    var sub = details.path || kindLabel(node.kind);
    panelHeader(panel, own(KIND_ICON, node.kind) || "package", node.title || node.id, sub, closePanel, node.group ? "group-" + node.group : "kind-" + node.kind);
    var tags = html("p", { class: "tags" }, panel);
    if (node.group) html("span", { class: "tag group-tag group-" + node.group }, tags, groupLabel(node.group) + (node.subgroup ? " · " + node.subgroup : ""));
    if (details.pkg) html("span", { class: "tag" }, tags, details.pkg.command ? "command" : "library");
    if (details.pkg && details.pkg.testSupport) html("span", { class: "tag" }, tags, "test support");
    node.alsoIn.forEach(function (g) {
      html("span", { class: "tag" }, tags, "also in " + groupLabel(g));
    });
    var style = changeStyle(details.change);
    if (style) {
      var banner = html("div", { class: "change-banner " + style.className }, panel);
      html("strong", {}, banner, style.label);
      var by = details.proposal ? " by “" + details.proposal.title + "”" : "";
      var what = details.changed.length ? ": " + details.changed.join(", ") : "";
      html("span", {}, banner, by + what + ".");
      if (details.change === "changed" && node.was && node.was.summary && node.changed.indexOf("summary") >= 0) {
        html("p", { class: "was" }, banner, "Was: " + node.was.summary.text);
      }
    }
    var actions = html("div", { class: "actions" }, panel);
    var here = app.state.view;
    if (node.kind === "system") {
      actionButton(actions, "inside", "Open containers", function () { go({ view: "containers", node: id }, { anchor: id }); }, true);
    } else if (details.contains.length && !(here === "inside" && app.state.of === id)) {
      actionButton(actions, "inside", "Look inside", function () { go({ view: "inside", of: id }, { anchor: id }); }, true);
    }
    if (!(here === "focus" && app.state.of === id)) {
      actionButton(actions, "focus", "Focus view", function () { go({ view: "focus", of: id, node: id }, { anchor: id }); }, !details.contains.length && node.kind !== "system");
    }
    if (node.group && !(here === "group" && app.state.of === node.group)) {
      actionButton(actions, "group", "Show in " + groupLabel(node.group), function () { go({ view: "group", of: node.group, node: id }); });
    }

    renderStatement(node.summary, section(panel, "Summary", undefined, "summary"), "lead");
    if (details.pkg && details.pkg.doc) {
      html("p", { class: "package-doc" }, section(panel, "Package doc", undefined, "doc"), details.pkg.doc);
    }
    renderStatements(panel, "does", "What it does", node.does);
    renderStatements(panel, "owns", "Data it owns", node.owns);
    renderStatements(panel, "interfaces", "Interfaces", node.interfaces);
    if (details.path || details.callers.length || details.dependencies.length) {
      renderRelations(panel, "Who calls it", details.callers, "Nothing in the module imports it.", "callers");
      renderRelations(panel, "Depends on", details.dependencies, "It imports no other package of the module.", "dependencies");
    }
    if (details.incoming.length) renderRelations(panel, "Reached by", details.incoming, "", "incoming");
    if (details.outgoing.length) renderRelations(panel, "Reaches", details.outgoing, "", "outgoing");
    if (details.contains.length) renderRelations(panel, "Contains", details.contains.map(function (c) { return { id: c.id, title: c.title }; }), "", "contains");
    if (details.containedBy.length) renderRelations(panel, "Part of", details.containedBy.map(function (c) { return { id: c.id, title: c.title }; }), "", "contained-by");

    var termSection = section(panel, "Glossary terms", details.terms.length || undefined, "terms");
    if (!details.terms.length) {
      html("p", { class: "empty" }, termSection, "None named yet.");
    } else {
      var dl = html("dl", { class: "terms" }, termSection);
      details.terms.forEach(function (t) {
        var dt = html("dt", {}, dl);
        var tb = html("button", { type: "button", class: "linklike term-name" }, dt, t.term);
        tb.addEventListener("click", function () {
          select(undefined, { term: t.term });
        });
        html("dd", {}, dl, t.definition || "Not in GLOSSARY.md.");
        if (t.avoid.length) html("dd", { class: "avoid" }, dl, "Avoid: " + t.avoid.join(", "));
      });
    }
    renderStatements(panel, "gotchas", "Gotchas", node.gotchas);

    if (details.facts.length) {
      details.facts.forEach(function (group) {
        var fs = section(panel, group.title, group.items.length, "facts-" + group.key);
        var ul = html("ul", { class: "facts" }, fs);
        group.items.forEach(function (item) {
          var hit = app.flash && factMatches(app.flash, group.key, item.title);
          var li = html("li", { class: hit ? "flash" : null }, ul);
          html("code", {}, li, item.title);
          item.detail.forEach(function (line) {
            html("div", { class: "fact-detail" }, li, line);
          });
        });
      });
    } else if (details.path) {
      html("p", { class: "empty" }, section(panel, "Routes, commands, and tables", undefined, "facts"), "None extracted for this package.");
    }

    var flowSection = section(panel, "Flows it takes part in", details.flows.length || undefined, "flows");
    if (!details.flows.length) {
      html("p", { class: "empty" }, flowSection, "None yet.");
    } else {
      var fl = html("ul", { class: "flow-list" }, flowSection);
      details.flows.forEach(function (f) {
        var li = html("li", {}, fl);
        var b = html("button", { type: "button", class: "flow-link" }, li);
        htmlIcon("flow", b);
        html("span", {}, b, f.title);
        b.addEventListener("click", function () {
          go({ view: "flow", of: f.id, step: f.steps[0] || 1 });
        });
        var steps = html("span", { class: "step-chips" }, li);
        f.steps.slice(0, 12).forEach(function (n) {
          var sb = html("button", { type: "button", class: "step-chip", "aria-label": "Step " + n + " of " + f.title }, steps, String(n));
          sb.addEventListener("click", function () {
            go({ view: "flow", of: f.id, step: n });
          });
        });
        if (f.steps.length > 12) html("span", { class: "more" }, steps, "+" + (f.steps.length - 12));
      });
    }
    if (details.path) renderPackageHistory(panel, details.history);
    if (details.diagrams.length) {
      var ds = section(panel, "Diagrams", details.diagrams.length, "diagrams");
      var dlist = html("ul", { class: "flow-list" }, ds);
      details.diagrams.forEach(function (g) {
        var li = html("li", {}, dlist);
        var b = html("button", { type: "button", class: "flow-link" }, li);
        htmlIcon(g.kind === "er" ? "table" : g.kind === "state" ? "state" : g.kind === "sequence" ? "flow" : "diagram", b);
        html("span", {}, b, g.title);
        html("span", { class: "diagram-kind" }, li, { graph: "graph", sequence: "sequence", state: "state", er: "tables" }[g.kind] || g.kind);
        b.addEventListener("click", function () {
          go({ view: "diagram", of: g.id, node: id });
        });
      });
    }
    if (node.links.length) {
      var ls = section(panel, "Links", node.links.length, "links");
      var ul = html("ul", { class: "sources block" }, ls);
      node.links.forEach(function (ref) {
        renderRef(ref, ul, ls);
      });
    }
  }

  // renderPackageHistory lists the History entries that touched a package;
  // each opens in the History view.
  function renderPackageHistory(panel, entries) {
    var hs = section(panel, "History", entries.length || undefined, "history");
    if (!entries.length) {
      html("p", { class: "empty" }, hs, "No History entry touches it yet.");
      return;
    }
    var ul = html("ul", { class: "history-links" }, hs);
    entries.forEach(function (e) {
      var b = html("button", { type: "button", class: "history-link" }, html("li", {}, ul));
      htmlIcon("history", b);
      html("span", { class: "history-link-title" }, b, e.title);
      html("time", { class: "history-link-date", datetime: e.date }, b, e.dateLabel);
      b.addEventListener("click", function () {
        go({ view: "history", entry: e.id });
      });
    });
  }

  function factMatches(flash, key, title) {
    if (flash.type === "table") return key === "databases" && title === flash.value;
    if (flash.type === "route") return key === "routes" && (title === flash.value || title.split(" ").pop() === flash.value);
    if (flash.type === "command") return key === "commands" && title.indexOf(flash.value) === 0;
    return false;
  }

  function renderTermPanel(panel, term) {
    var entry = app.data.termByName[term];
    panelHeader(panel, "term", term, "Glossary term", function () {
      select(app.state.node, { term: undefined });
    });
    var s = section(panel, "Definition", undefined, "definition");
    html("p", { class: "definition" }, s, entry ? entry.definition : "");
    if (entry && list(entry.avoid).length) html("p", { class: "avoid" }, s, "Avoid: " + entry.avoid.join(", "));
    var a = html("p", { class: "sources" }, s);
    var link = resolveRef("doc:GLOSSARY.md", app.data, app.links);
    var anchor = html("a", { href: link.href, target: "_blank", rel: "noopener" }, a);
    htmlIcon("doc", anchor, "ref-icon");
    html("span", {}, anchor, "GLOSSARY.md");
    var users = termUsers(app.data, term);
    renderRelations(panel, "Used by", users.map(function (u) { return { id: u.id, title: u.title }; }), "No node names this term yet.", "users");
  }

  function renderFlowPanel(panel) {
    var flow = null;
    if (app.state.view === "flow") {
      app.data.flows.forEach(function (f) {
        if (f.id === app.state.of) flow = f;
      });
    } else {
      var found = findDiagram(app.data, app.state.of);
      flow = found && found.diagram;
    }
    if (!flow) return;
    var style = changeStyle(flow.change);
    panelHeader(panel, "flow", flow.title || flow.id, app.state.view === "flow" ? "Flow · " + flow.steps.length + " steps" : "Sequence diagram", function () {
      go({ view: app.state.view === "flow" ? "flows" : "components" });
    });
    if (style) {
      var banner = html("div", { class: "change-banner " + style.className }, panel);
      html("strong", {}, banner, style.label);
      html("span", {}, banner, " by a design proposal.");
    }
    var current = app.state.step || 1;
    var step = app.layout.steps[current - 1];
    if (step) {
      var card = html("section", { class: "step-card" }, panel);
      var head = html("div", { class: "step-card-head" }, card);
      html("span", { class: "step-badge" }, head, String(step.number));
      html("span", { class: "step-of" }, head, "Step " + step.number + " of " + app.layout.steps.length);
      html("span", { class: "tag" }, head, step.kind + (step.via ? " · " + step.via : ""));
      var route = html("p", { class: "step-route" }, card);
      var fromB = html("button", { type: "button", class: "linklike" }, route, titleOf(app.data, step.from));
      fromB.addEventListener("click", function () { jumpToNode(step.from); });
      html("span", { class: "arrow", "aria-hidden": "true" }, route, step.kind === "note" ? " · " : step.kind === "return" ? " ⇠ " : " → ");
      if (step.to !== step.from || step.kind === "note") {
        var toB = html("button", { type: "button", class: "linklike" }, route, titleOf(app.data, step.to));
        toB.addEventListener("click", function () { jumpToNode(step.to); });
      } else {
        html("span", {}, route, "itself");
      }
      renderStatement({ text: step.label, refs: step.refs }, card, "lead");
      var nav = html("div", { class: "step-nav" }, card);
      var prev = actionButton(nav, "prev", "Previous", function () { setStep(current - 1); });
      prev.disabled = current <= 1;
      var next = actionButton(nav, "next", "Next", function () { setStep(current + 1); }, true);
      next.disabled = current >= app.layout.steps.length;
    }
    if (flow.summary) renderStatement(flow.summary, section(panel, "Summary", undefined, "summary"));
    if (flow.trigger) renderStatement(flow.trigger, section(panel, "Starts when", undefined, "trigger"));
    renderRelations(panel, "Participants", list(flow.participants).map(function (pid) {
      return { id: pid, title: titleOf(app.data, pid) };
    }), "", "participants");
    var all = section(panel, "Steps", app.layout.steps.length, "steps");
    var ol = html("ol", { class: "step-list" }, all);
    app.layout.steps.forEach(function (s) {
      var li = html("li", { class: s.number === current ? "current" : null }, ol);
      var b = html("button", { type: "button", "aria-current": s.number === current ? "step" : null }, li);
      html("span", { class: "n" }, b, String(s.number));
      html("span", { class: "who" }, b, titleOf(app.data, s.from) + (s.kind === "note" ? " · " : " → ") + titleOf(app.data, s.to));
      html("span", { class: "what" }, b, s.label);
      b.addEventListener("click", function () {
        setStep(s.number);
      });
    });
  }

  function renderProposalPanel(panel) {
    panelHeader(panel, "diagram", "Design proposals", list(app.state.proposals).length + " on", closePanel);
    list(app.state.proposals).forEach(function (pid) {
      var p = proposalDetails(app.data, pid);
      if (!p) return;
      var box = html("section", { class: "proposal-card" }, panel);
      var head = html("div", { class: "proposal-head" }, box);
      html("h3", {}, head, p.title);
      html("span", { class: "status status-" + p.status }, head, p.status);
      if (p.spec) {
        var specList = html("ul", { class: "sources" }, box);
        renderRef(p.spec, specList, box);
      }
      renderStatement(p.summary, box, "lead");
      var counts = { add: 0, change: 0, remove: 0 };
      p.operations.forEach(function (o) {
        counts[o.op] = (counts[o.op] || 0) + 1;
      });
      var summary = html("p", { class: "op-counts" }, box);
      [["add", "added"], ["change", "changed"], ["remove", "removed"]].forEach(function (pair) {
        html("span", { class: "change-pill change-" + pair[1] }, summary, counts[pair[0]] + " " + pair[1]);
      });
      renderStatements(box, "requirements-" + pid, "Requirements", p.requirements);
      renderStatements(box, "decisions-" + pid, "Decisions", p.decisions);
      var ops = section(box, "What it changes", p.operations.length, "operations");
      var ul = html("ul", { class: "op-list" }, ops);
      p.operations.forEach(function (o) {
        var change = o.op === "add" ? "added" : o.op === "change" ? "changed" : "removed";
        var li = html("li", { class: "change-" + change }, ul);
        var b = html("button", { type: "button", class: "op" }, li);
        html("span", { class: "change-pill change-" + change }, b, o.op);
        html("span", { class: "op-what" }, b, o.what);
        html("span", { class: "op-label" }, b, o.label);
        b.addEventListener("click", function () {
          if (o.target.indexOf("flow:") === 0) go({ view: "flow", of: o.target.slice(5), step: 1 });
          else jumpToNode(o.target);
        });
      });
    });
  }

  // ------------------------------------------------------------------ boot

  function wireChrome() {
    var zin = byId("zoom-in");
    var zout = byId("zoom-out");
    var zfit = byId("zoom-fit");
    htmlIcon("zoom-in", zin);
    htmlIcon("zoom-out", zout);
    htmlIcon("fit", zfit);
    zin.addEventListener("click", function () {
      zoomBy(ZOOM_STEP);
    });
    zout.addEventListener("click", function () {
      zoomBy(1 / ZOOM_STEP);
    });
    zfit.addEventListener("click", fit);
    wireSearch();
    document.addEventListener("keydown", function (event) {
      var typing = event.target && /^(INPUT|TEXTAREA|SELECT)$/.test(event.target.tagName);
      if (typing || event.metaKey || event.ctrlKey || event.altKey) return;
      if (event.key === "/") {
        event.preventDefault();
        byId("search-input").focus();
      } else if (event.key === "Escape") {
        if (app.state.node || app.state.term || app.state.panel) closePanel();
      } else if ((event.key === "ArrowRight" || event.key === "ArrowDown") && stepCount()) {
        event.preventDefault();
        setStep((app.state.step || 1) + 1);
      } else if ((event.key === "ArrowLeft" || event.key === "ArrowUp") && stepCount()) {
        event.preventDefault();
        setStep((app.state.step || 1) - 1);
      } else if (event.key === "+" || event.key === "=") {
        zoomBy(ZOOM_STEP);
      } else if (event.key === "-" || event.key === "_") {
        zoomBy(1 / ZOOM_STEP);
      } else if (event.key === "0") {
        fit();
      }
    });
    window.addEventListener("popstate", function () {
      apply(parseHash(location.hash), {});
    });
    window.addEventListener("hashchange", function () {
      var raw = parseHash(location.hash);
      if (formatHash(raw) !== formatHash(app.raw)) apply(raw, {});
    });
    // When the canvas changes size (the window, or the panel opening or
    // closing), keep the drawing's scale and its top-left corner.
    function keepScale() {
      if (!app.box || !app.svg || !app.size) return;
      var size = canvasSize();
      if (size.w === app.size.w && size.h === app.size.h) return;
      var scale = app.size.w / app.box.width;
      setBox({ x: app.box.x, y: app.box.y, width: size.w / scale, height: size.h / scale });
    }
    if (typeof ResizeObserver === "function") new ResizeObserver(keepScale).observe(byId("canvas"));
    else window.addEventListener("resize", keepScale);
  }

  function boot(objects) {
    app.base = mergeData(objects);
    app.overlays = dict();
    renderProblems();
    wireChrome();
    var canvas = byId("canvas");
    Array.prototype.slice.call(canvas.querySelectorAll(".loading")).forEach(function (el) {
      el.parentNode.removeChild(el);
    });
    apply(parseHash(location.hash), { instant: true });
  }

  // The loader. A served page first loads build.js, which names the running
  // binary's build; if it does not load, links open the public repository. The manifest
  // lists data files relative to data/; each is loaded in order as a classic
  // script, which registers one object. Then the proposal query parameter
  // may name one more script inside the explorer directory, which may
  // register design proposals only.
  var registered = [];
  var booted = false;
  global.evieExplorer = {
    register: function (object) {
      registered.push(object);
    },
    // build.js calls this before the data loads; later calls change nothing.
    build: function (info) {
      if (app.base) return;
      app.links = pageLinks(location.protocol, info);
    },
    manifest: function (files) {
      if (booted) return;
      booted = true;
      var i = 0;
      function load(src, done) {
        var script = document.createElement("script");
        script.src = src;
        script.onload = function () {
          done(true);
        };
        script.onerror = function () {
          done(false);
        };
        document.head.appendChild(script);
      }
      function next() {
        if (i >= files.length) {
          extra();
          return;
        }
        var file = files[i++];
        load("data/" + file, function (ok) {
          if (!ok) app.problems.push("This data file did not load: data/" + file + ".");
          next();
        });
      }
      function extra() {
        var wanted = proposalPath(location.search);
        if (!wanted) {
          boot(registered);
          return;
        }
        if (wanted.error) {
          app.problems.push(wanted.error);
          boot(registered);
          return;
        }
        var before = registered.length;
        load(wanted.path, function (ok) {
          var added = registered.splice(before);
          if (!ok) app.problems.push("The design proposal file " + wanted.path + " did not load.");
          var known = dict();
          registered.forEach(function (o) {
            if (o && o.kind === "proposal") known[o.id] = true;
          });
          added.forEach(function (o) {
            if (!o || o.kind !== "proposal") {
              app.problems.push(wanted.path + " registered something other than a design proposal; it was ignored.");
            } else if (known[o.id]) {
              app.problems.push("The design proposal " + o.id + " in " + wanted.path + " is already loaded; the copy was ignored.");
            } else {
              known[o.id] = true;
              registered.push(o);
            }
          });
          boot(registered);
        });
      }
      var build = buildScript(location.protocol);
      if (build) load(build, next);
      else next();
    },
  };
})(typeof window !== "undefined" ? window : globalThis);
