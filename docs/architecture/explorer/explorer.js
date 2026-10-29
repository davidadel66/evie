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

  function groupLabel(id) {
    var g = GROUPS[groupIndex(id)];
    return g ? g.label : id;
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
    var byKey = {};
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

  function copy(object) {
    var out = {};
    Object.keys(object).forEach(function (key) {
      out[key] = object[key];
    });
    return out;
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

  // applyProposals returns the data as the active design proposals would
  // leave it, without touching the built data: added items carry change
  // "added", changed items carry change "changed" with the changed field
  // names and the item as built in "was", and removed items stay in place
  // with change "removed" so the page can strike them through. Proposals
  // apply in the order the data lists them; unknown ids are ignored.
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
    out.nodeById = copy(data.nodeById);
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
    added: { label: "Added", dashed: true, badge: "", struck: false, faded: false, legend: "Added by a proposal (dashed outline)" },
    changed: { label: "Changed", dashed: false, badge: "Δ", struck: false, faded: false, legend: "Changed by a proposal (Δ badge)" },
    removed: { label: "Removed", dashed: false, badge: "", struck: true, faded: true, legend: "Removed by a proposal (struck through)" },
  };
  var CHANGES = ["added", "changed", "removed"];

  function changeStyle(change) {
    var style = Object.prototype.hasOwnProperty.call(CHANGE_STYLES, change) ? CHANGE_STYLES[change] : null;
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
    return KIND_LABELS[kind] || kind || "";
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
    Object.keys(extra || {}).forEach(function (key) {
      out[key] = extra[key];
    });
    return out;
  }

  function emptyLayout(kind, title, text) {
    return { kind: kind, width: 0, height: 0, nodes: [], edges: [], lanes: [], empty: { title: title, text: text } };
  }

  // finishLayout gives the layout its extent and rounds every box.
  function finishLayout(layout, margin) {
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
        h = Math.max(h, e.labelY + 10);
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
    var byId = {};
    nodes.forEach(function (n) {
      byId[n.id] = n;
    });
    var routes = [];
    var slots = {};
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
    return routes.map(function (r) {
      var waypoints = opts.waypointsFor ? opts.waypointsFor(r.link, r.p1, r.p2, r.a, r.b) : [];
      var path = curvePath(r.p1, NORMALS[r.sides[0]], r.p2, NORMALS[r.sides[1]], waypoints, opts.axis || "h");
      var label = r.link.label || "";
      return {
        id: r.link.from + ">" + r.link.to + ">" + (r.link.via || ""),
        from: r.a.id,
        to: r.b.id,
        via: r.link.via || "",
        label: label,
        labelWidth: label ? round(textWidth(label, 11) + 12) : 0,
        change: r.link.change || "",
        faded: !!r.link.faded,
        x1: r.p1.x,
        y1: r.p1.y,
        x2: r.p2.x,
        y2: r.p2.y,
        d: path.d,
        labelX: round(path.mid.x),
        labelY: round(path.mid.y),
      };
    });
  }

  function byTitleThenId(a, b) {
    return byString(String(a.title || "").toLowerCase(), String(b.title || "").toLowerCase()) || byString(a.id, b.id);
  }

  // linksAmong lists the links whose two ends both have drawn nodes, in a
  // fixed order whatever order the data gave them in.
  function linksAmong(data, ids) {
    var set = {};
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

  var CONTEXT = { margin: 40, columnGap: 170, rowGap: 30, nodeHeight: 78, minWidth: 210, systemWidth: 260, systemMinHeight: 160 };

  // layoutContext arranges the system context by role: people on the left,
  // the system in the middle, and the outside systems it talks to on the
  // right.
  function layoutContext(data) {
    var c = CONTEXT;
    var nodes = data.nodes.filter(function (n) {
      return n.level === "context";
    });
    if (!nodes.length) {
      return emptyLayout("context", "No system context yet", "The context view fills in once content names the owner and the outside systems Evie talks to.");
    }
    nodes.sort(byTitleThenId);
    var columns = [[], [], []];
    nodes.forEach(function (node) {
      var col = node.kind === "person" ? 0 : node.kind === "system" ? 1 : 2;
      var width = Math.max(col === 1 ? c.systemWidth : c.minWidth, textWidth(node.title, 15) + 72);
      columns[col].push(drawnNode(node, width, c.nodeHeight));
    });
    var outer = Math.max(columns[0].length, columns[2].length);
    var tall = outer * c.nodeHeight + Math.max(outer - 1, 0) * c.rowGap;
    columns[1].forEach(function (n) {
      n.height = round(Math.max(c.systemMinHeight, (tall - (columns[1].length - 1) * c.rowGap) / columns[1].length * 0.7));
    });
    stackColumns(columns, c.margin, c.margin, c.columnGap, c.rowGap);
    var drawn = columns[0].concat(columns[1], columns[2]);
    var layout = {
      kind: "context",
      nodes: drawn,
      edges: routeLinks(drawn, linksAmong(data, drawn.map(function (n) { return n.id; })), { portGap: 18 }),
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
    laneGap: 120,
    lanePad: 20,
    laneHeader: 42,
    rowGap: 30,
    nodeHeight: 66,
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
    var neighbours = {};
    links.forEach(function (l) {
      if (l.from === l.to) return;
      (neighbours[l.from] = neighbours[l.from] || []).push(l.to);
      (neighbours[l.to] = neighbours[l.to] || []).push(l.from);
    });
    function positions() {
      var pos = {};
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
  function placeRow(row, wanted, gap) {
    var offset = 0;
    var targets = row.map(function (n, i) {
      var t = wanted[i] - n.width / 2 - offset;
      offset += n.width + gap;
      return t;
    });
    var z = isotonic(targets);
    offset = 0;
    row.forEach(function (n, i) {
      n.x = z[i] + offset;
      offset += n.width + gap;
    });
  }

  function crossingCount(upper, lower, segments) {
    var pu = {};
    var pl = {};
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

  var LAYERED = { rankGap: 70, nodeGap: 28, dummyWidth: 8, sweeps: 16, placeRounds: 10, portGap: 14 };

  // layered lays out boxes ({id, width, height, last}) joined by links
  // ({from, to}) from top to bottom. Boxes marked last share the bottom
  // layer. It sets each box's x and y and returns one route per link: the
  // points it passes through and the sides it leaves and arrives on.
  function layered(boxes, links, opts) {
    opts = opts || {};
    var rankGap = opts.rankGap || LAYERED.rankGap;
    var nodeGap = opts.nodeGap || LAYERED.nodeGap;
    var byId = {};
    var ids = boxes
      .map(function (b) {
        byId[b.id] = b;
        return b.id;
      })
      .sort(byString);
    var out = {};
    var inn = {};
    ids.forEach(function (id) {
      out[id] = [];
      inn[id] = [];
    });
    var edges = [];
    links.forEach(function (link, index) {
      if (!byId[link.from] || !byId[link.to]) return;
      edges.push({ link: link, index: index, from: link.from, to: link.to, self: link.from === link.to });
    });
    edges.forEach(function (e) {
      if (!e.self) out[e.from].push(e);
    });
    // Break cycles: a depth-first search in id order reverses the edges that
    // lead back to a box on its stack.
    var state = {};
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
    ids.forEach(function (id) {
      if (!state[id]) visit(id);
    });
    var dag = edges.filter(function (e) {
      return !e.self;
    });
    dag.forEach(function (e) {
      e.top = e.reversed ? e.to : e.from;
      e.bottom = e.reversed ? e.from : e.to;
    });
    ids.forEach(function (id) {
      out[id] = [];
      inn[id] = [];
    });
    dag.forEach(function (e) {
      out[e.top].push(e);
      inn[e.bottom].push(e);
    });
    // Rank by longest path from the sources, in topological order.
    var rank = {};
    var pending = {};
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
    var maxRank = 0;
    ids.forEach(function (id) {
      if (rank[id] === undefined) rank[id] = 0;
      if (!byId[id].last) maxRank = Math.max(maxRank, rank[id]);
    });
    var hasLast = ids.some(function (id) {
      return byId[id].last;
    });
    ids.forEach(function (id) {
      if (byId[id].last) rank[id] = maxRank + 1;
    });
    if (hasLast) maxRank += 1;
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
        var dummy = { id: "~" + e.index + "~" + k, width: LAYERED.dummyWidth, height: 0, dummy: true };
        byId[dummy.id] = dummy;
        rows[k].push(dummy);
        chain.push(dummy.id);
      }
      chain.push(e.bottom);
      e.chain = chain;
      for (var i = 0; i + 1 < chain.length; i++) segments.push([chain[i], chain[i + 1]]);
    });
    var up = {};
    var down = {};
    segments.forEach(function (s) {
      (down[s[0]] = down[s[0]] || []).push(s[1]);
      (up[s[1]] = up[s[1]] || []).push(s[0]);
    });
    rows.forEach(function (row) {
      row.sort(function (a, b) {
        return (a.dummy ? 1 : 0) - (b.dummy ? 1 : 0) || byString(a.sortKey || a.id, b.sortKey || b.id);
      });
    });
    function total(rs) {
      var c = 0;
      for (var i = 0; i + 1 < rs.length; i++) c += crossingCount(rs[i], rs[i + 1], segments);
      return c;
    }
    function reorder(rs, i, neighbours, ref) {
      var pos = {};
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
    rows.forEach(function (row) {
      var x = 0;
      row.forEach(function (n) {
        n.x = x;
        x += n.width + nodeGap;
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
      placeRow(row, wanted, nodeGap);
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
    var bottomPorts = {};
    var topPorts = {};
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
    var out = {
      id: link.from + ">" + link.to + ">" + (link.via || ""),
      from: from,
      to: to,
      via: link.via || "",
      label: label,
      labelWidth: label ? round(textWidth(label, 11) + 12) : 0,
      change: link.change || "",
      faded: !!link.faded,
      x1: a.x,
      y1: a.y,
      x2: b.x,
      y2: b.y,
      d: path.d,
      labelX: round(path.mid.x + (route.self ? 14 : 0)),
      labelY: round(path.mid.y),
    };
    Object.keys(extra || {}).forEach(function (key) {
      out[key] = extra[key];
    });
    if (route.self) out.extent = [{ x: a.x + 60, y: b.y }];
    return out;
  }

  // ------------------------------------------------------------- group view

  var GROUP_VIEW = { margin: 36, nodeHeight: 54, chipHeight: 38, minWidth: 140, rankGap: 64, nodeGap: 26 };

  function packageBox(node, data, boundary) {
    var path = packagePath(node.id);
    var subtitle = boundary ? groupLabel(node.group) || "Outside" : path || kindLabel(node.kind);
    var width = Math.max(GROUP_VIEW.minWidth, textWidth(node.title, 14) + 34, textWidth(subtitle, 11) + 34);
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
  // depend on outside the set as faded chips along the bottom.
  function layoutMembers(data, kind, memberIds, title) {
    var g = GROUP_VIEW;
    var member = {};
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
    var outside = {};
    var links = [];
    data.links.forEach(function (l) {
      if (!member[l.from] || l.from === l.to) return;
      if (member[l.to]) {
        links.push(l);
      } else if (data.nodeById[l.to] && data.nodeById[l.to].level === "component") {
        outside[l.to] = true;
        var faded = copy(l);
        faded.faded = true;
        links.push(faded);
      }
    });
    Object.keys(outside)
      .sort(byString)
      .forEach(function (id) {
        boxes.push(packageBox(data.nodeById[id], data, true));
      });
    links.sort(function (a, b) {
      return byString(linkKey(a), linkKey(b));
    });
    var result = layered(boxes, links, { rankGap: g.rankGap, nodeGap: g.nodeGap });
    boxes.forEach(function (b) {
      b.x = round(b.x + g.margin);
      b.y = round(b.y + g.margin);
      delete b.last;
      delete b.sortKey;
    });
    var edges = result.routes.map(function (route) {
      route.points = route.points.map(function (p) {
        return { x: round(p.x + g.margin), y: round(p.y + g.margin) };
      });
      return layeredEdge(route, route.link.from, route.link.to);
    });
    return finishLayout({ kind: kind, title: title, nodes: boxes, edges: edges, lanes: [], empty: null }, g.margin);
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

  // The address fragment holds the view and its subject ("of": a group, a
  // node, a flow, or a diagram), the selected node or glossary term, the
  // flow step, the panel, and the active proposals.
  var HASH_KEYS = ["view", "of", "node", "term", "step", "panel", "proposals"];

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
        return { error: "The proposal parameter is not a valid escape sequence." };
      }
      if (!PROPOSAL_PATH.test(value)) {
        return {
          error:
            "The proposal parameter must be a relative path to a .js file inside the explorer directory, such as examples/design-proposal.js; absolute paths, schemes, and .. are refused.",
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
    textWidth: textWidth,
    wrapText: wrapText,
    packageDetails: packageDetails,
    factsForPackage: factsForPackage,
    resolveRef: resolveRef,
    formatHash: formatHash,
    parseHash: parseHash,
    proposalPath: proposalPath,
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
