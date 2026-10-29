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
    var counts = {};
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
    var panelById = {};
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

  // rankEdges ranks ids by longest path along edges ({from, to, index,
  // self}). A depth-first search first reverses the edges that lead back to
  // an id on its stack, so cycles rank too; each edge gets
  // reversed, top, and bottom. Self edges are left out of the ranking.
  function rankEdges(ids, edges) {
    var out = {};
    var inn = {};
    ids.forEach(function (id) {
      out[id] = [];
      inn[id] = [];
    });
    edges.forEach(function (e) {
      if (!e.self) out[e.from].push(e);
    });
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
    // Sources first, so a cycle is broken where the walk from a source
    // closes it, not at an arbitrary id.
    var indegree = {};
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
    var byId = {};
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
      var balance = {};
      dag.forEach(function (e) {
        balance[e.top] = (balance[e.top] || 0) + 1;
        balance[e.bottom] = (balance[e.bottom] || 0) - 1;
      });
      var next = 0;
      var remap = {};
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
    var up = {};
    var down = {};
    segments.forEach(function (s) {
      (down[s[0]] = down[s[0]] || []).push(s[1]);
      (up[s[1]] = up[s[1]] || []).push(s[0]);
    });
    // Start each row with its boxes in name order and its dummies spread
    // evenly between them, so edges passing a wrapped layer go between its
    // boxes rather than around one end.
    var relative = {};
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
      refs: list(link.refs),
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

  var GROUP_VIEW = { margin: 36, nodeHeight: 54, chipHeight: 38, minWidth: 140, rankGap: 64, nodeGap: 26, rowBudget: 1400, chipGap: 12, chipRowGap: 12 };

  function packageBox(node, data, boundary) {
    var path = packagePath(node.id);
    var subtitle = boundary ? (node.group ? groupLabel(node.group) : kindLabel(node.kind)) : path || kindLabel(node.kind);
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
  // depend on outside the set as faded chips in rows below, each chip under
  // the members that use it. Edges to chips are drawn on demand: when a
  // member is hovered or selected.
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
    var byId = {};
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

  var FOCUS = { margin: 36, columnGap: 150, chipHeight: 44, chipGap: 12, minChip: 170, focusWidth: 300, header: 34, portGap: 14 };

  // layoutFocus draws one node with who calls it on the left and what it
  // depends on on the right, from the import graph and the content edges.
  function layoutFocus(data, id) {
    var f = FOCUS;
    var node = data.nodeById[id];
    if (!node) return emptyLayout("focus", "No such node", "The address names a node the data does not have.");
    var seen = { callers: {}, dependencies: {} };
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
          return drawnNode(n, Math.max(f.minChip, textWidth(n.title, 13) + 40, textWidth(subtitle, 11) + 40), f.chipHeight, {
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
      edges: routeLinks(nodes, links, { portGap: f.portGap }),
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
    minHead: 130,
    headGap: 48,
    labelMax: 230,
    labelSize: 12,
    lineHeight: 15,
    rowPad: 30,
    selfReach: 44,
    selfDrop: 26,
    noteMax: 260,
    notePad: 36,
    firstRow: 34,
    tail: 34,
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
    var index = {};
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
        width: round(Math.max(q.minHead, textWidth(title, 13.5) + 32, textWidth(subtitle, 11) + 32)),
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
      var drawn = {
        number: l.s.number,
        from: step.from,
        to: step.to,
        kind: l.kind,
        via: step.via || "",
        label: step.label || "",
        lines: l.lines,
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
        var dir = drawn.x2 > drawn.x1 ? 1 : -1;
        drawn.d =
          "M" + drawn.x1 + "," + drawn.y +
          " C" + round(drawn.x1 + dir * 10) + "," + drawn.y + " " + round(drawn.x2 - dir * 10) + "," + drawn.y + " " + drawn.x2 + "," + drawn.y;
        drawn.labelX = round((drawn.x1 + drawn.x2) / 2);
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

  var DIAGRAM = { margin: 32, nodeHeight: 46, minWidth: 110, rankGap: 74, nodeGap: 40, loopRoom: 60 };

  function diagramBox(id, title, ref, data) {
    var target = ref ? data.nodeById[ref] : null;
    return {
      id: id,
      ref: target ? ref : null,
      title: title,
      subtitle: target ? target.title : "",
      kind: target ? target.kind || "" : "",
      group: target ? target.group || "" : "",
      change: target ? target.change || "" : "",
      x: 0,
      y: 0,
      width: round(Math.max(DIAGRAM.minWidth, textWidth(title, 13) + 32)),
      height: DIAGRAM.nodeHeight,
    };
  }

  // layoutLayeredDiagram lays out a plain graph or a state diagram top to
  // bottom with the layered engine.
  function layoutLayeredDiagram(kind, boxes, links, title) {
    var d = DIAGRAM;
    var selfLoops = {};
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
    var known = {};
    var boxes = list(diagram.nodes).map(function (n) {
      known[n.id] = true;
      return diagramBox(n.id, n.label || n.id, n.node, data);
    });
    if (!boxes.length) return emptyLayout("graph", "Nothing to draw yet", "This diagram lists no nodes.");
    var links = graphLinks(diagram, known);
    if (list(diagram.lanes).length) return layoutLaneGraph(data, diagram, boxes, links);
    return layoutLayeredDiagram("graph", boxes, links, diagram.title);
  }

  var LANES = { margin: 28, header: 132, pad: 18, colGap: 96, rowGap: 18, bandGap: 10, passPad: 6 };

  // layoutLaneGraph draws a graph with lanes as horizontal bands, its nodes
  // flowing left to right by longest path, each in its lane's band.
  function layoutLaneGraph(data, diagram, boxes, links) {
    var g = LANES;
    var byId = {};
    var ids = boxes
      .map(function (b) {
        byId[b.id] = b;
        return b.id;
      })
      .sort(byString);
    var edges = links.map(function (l, index) {
      return { link: l, index: index, from: l.from, to: l.to, self: l.from === l.to };
    });
    var rank = rankEdges(ids, edges);
    var laneDefs = list(diagram.lanes).map(function (l) {
      return { id: l.id, title: l.title || l.id };
    });
    var laneOf = {};
    list(diagram.nodes).forEach(function (n) {
      laneOf[n.id] = n.lane || "";
    });
    var laneIndex = {};
    laneDefs.forEach(function (l, i) {
      laneIndex[l.id] = i;
    });
    if (ids.some(function (id) { return laneIndex[laneOf[id]] === undefined; })) {
      laneIndex[""] = laneDefs.length;
      laneDefs.push({ id: "", title: "Other" });
    }
    var maxRank = 0;
    ids.forEach(function (id) {
      maxRank = Math.max(maxRank, rank[id]);
    });
    var cells = laneDefs.map(function () {
      var row = [];
      for (var r = 0; r <= maxRank; r++) row.push([]);
      return row;
    });
    ids.forEach(function (id) {
      var li = laneIndex[laneOf[id]] === undefined ? laneIndex[""] : laneIndex[laneOf[id]];
      cells[li][rank[id]].push(byId[id]);
    });
    var colWidth = [];
    for (var r = 0; r <= maxRank; r++) {
      colWidth[r] = 0;
      cells.forEach(function (row) {
        row[r].forEach(function (b) {
          colWidth[r] = Math.max(colWidth[r], b.width);
        });
      });
    }
    var colX = [];
    var x = g.margin + g.header + g.pad;
    for (r = 0; r <= maxRank; r++) {
      colX[r] = x;
      x += colWidth[r] + g.colGap;
    }
    var right = x - g.colGap + g.pad;
    var y = g.margin;
    var lanes = laneDefs.map(function (def, li) {
      var tallest = 0;
      cells[li].forEach(function (cell) {
        cell.sort(function (a, b) {
          return byString(a.title, b.title) || byString(a.id, b.id);
        });
        tallest = Math.max(tallest, cell.reduce(function (h, b, k) {
          return h + b.height + (k ? g.rowGap : 0);
        }, 0));
      });
      var height = tallest + 2 * g.pad;
      cells[li].forEach(function (cell, rr) {
        var h = cell.reduce(function (sum, b, k) {
          return sum + b.height + (k ? g.rowGap : 0);
        }, 0);
        var cy = y + g.pad + (tallest - h) / 2;
        cell.forEach(function (b) {
          b.x = round(colX[rr] + (colWidth[rr] - b.width) / 2);
          b.y = round(cy);
          cy += b.height + g.rowGap;
        });
      });
      var lane = {
        id: def.id || "~other",
        title: def.title,
        x: g.margin,
        y: round(y),
        width: round(right - g.margin),
        height: round(height),
        nodes: [],
      };
      cells[li].forEach(function (cell) {
        cell.forEach(function (b) {
          lane.nodes.push(b.id);
        });
      });
      y += height + g.bandGap;
      return lane;
    });
    var columns = [];
    for (r = 0; r <= maxRank; r++) {
      var colBoxes = [];
      cells.forEach(function (row) {
        colBoxes = colBoxes.concat(row[r]);
      });
      columns.push({ x: colX[r], width: colWidth[r], y: g.margin, height: y - g.margin, boxes: colBoxes });
    }
    var laneAt = {};
    lanes.forEach(function (l) {
      l.nodes.forEach(function (id) {
        laneAt[id] = l;
      });
    });
    var forward = [];
    var special = [];
    edges.forEach(function (e) {
      if (!e.self && rank[e.to] > rank[e.from]) forward.push(e.link);
      else special.push(e);
    });
    var drawn = routeLinks(boxes, forward, {
      portGap: 12,
      waypointsFor: function (link, p1, p2) {
        return passThrough(columns, p1, p2, g.passPad);
      },
    });
    special.forEach(function (e) {
      var a = byId[e.from];
      var b = byId[e.to];
      var route;
      if (e.self) {
        route = { self: true, link: e.link, points: [
          { x: round(a.x + a.width), y: round(a.y + a.height * 0.3) },
          { x: round(a.x + a.width), y: round(a.y + a.height * 0.7) },
        ] };
        drawn.push(layeredEdge(route, e.from, e.to));
        return;
      }
      if (rank[e.to] === rank[e.from]) {
        drawn = drawn.concat(routeLinks(boxes, [e.link], { axis: "v" }));
        return;
      }
      // A return to an earlier node runs back along the bottom of the lower
      // node's band, below every node in it.
      var lower = laneAt[e.from].y >= laneAt[e.to].y ? laneAt[e.from] : laneAt[e.to];
      var corridor = round(lower.y + lower.height - g.pad / 2);
      var p1 = { x: round(a.x + a.width / 2), y: round(a.y + a.height) };
      var p2 = { x: round(b.x + b.width / 2), y: round(b.y + b.height) };
      var path = curvePath(p1, NORMALS.bottom, p2, NORMALS.bottom, [
        { x: p1.x, y: corridor },
        { x: p2.x, y: corridor },
      ], "h");
      var label = e.link.label || "";
      drawn.push({
        id: e.link.from + ">" + e.link.to + ">",
        from: e.from,
        to: e.to,
        via: "",
        label: label,
        labelWidth: label ? round(textWidth(label, 11) + 12) : 0,
        change: "",
        faded: false,
        refs: list(e.link.refs),
        x1: p1.x,
        y1: p1.y,
        x2: p2.x,
        y2: p2.y,
        d: path.d,
        labelX: round((p1.x + p2.x) / 2),
        labelY: corridor,
        back: true,
      });
    });
    drawn.sort(function (p, q) {
      return byString(p.id, q.id);
    });
    return finishLayout({ kind: "graph", title: diagram.title, nodes: boxes, edges: drawn, lanes: lanes, empty: null }, g.margin);
  }

  // layoutStateDiagram draws states top to bottom from the initial marker,
  // with every transition, loops and returns included.
  function layoutStateDiagram(data, diagram) {
    var known = {};
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
    var tables = {};
    list(db.tables).forEach(function (t) {
      tables[t.name] = t;
    });
    var owner = db.package && data.nodeById["pkg:" + db.package] ? "pkg:" + db.package : null;
    var cards = {};
    var names = [];
    function card(name, stub) {
      if (cards[name]) return cards[name];
      var table = tables[name];
      var fkCols = {};
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
      var width = Math.max(e.minWidth, textWidth(name, 14) + 48);
      columns.forEach(function (c) {
        width = Math.max(width, textWidth(c.name, 12) + textWidth(c.type, 11) + 74);
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
  // fact that names its package, and what an active proposal changes.
  function nodeDetails(data, id) {
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
    };
  }

  // proposalDetails gathers what the panel shows for a design proposal: its
  // statements and one line per operation, each with the node or flow to
  // jump to.
  function proposalDetails(data, id) {
    var p = proposalById(data, id);
    if (!p) return null;
    var titles = {};
    Object.keys(data.nodeById).forEach(function (k) {
      titles[k] = titleOf(data, k);
    });
    list(p.nodes).forEach(function (op) {
      if (op.op === "add" && op.id) titles[op.id] = op.title || op.id;
    });
    var flowTitles = {};
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

  var SEARCH_ORDER = { node: 0, term: 1, flow: 2, diagram: 3 };

  // search finds nodes by title or package path, glossary terms by term or
  // avoided synonym, flows, and diagrams by title; best matches first, each
  // with the address state that shows it.
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
    results.sort(function (a, b) {
      return b.score - a.score || SEARCH_ORDER[a.type] - SEARCH_ORDER[b.type] || byString(a.label.toLowerCase(), b.label.toLowerCase()) || byString(a.id, b.id);
    });
    return results.slice(0, limit || 12).map(function (r) {
      delete r.score;
      return r;
    });
  }

  // ------------------------------------------------------------ navigation

  var LEVELS = [
    { id: "context", label: "Context" },
    { id: "containers", label: "Containers" },
    { id: "components", label: "Components" },
    { id: "flows", label: "Flows" },
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
  };

  function levelOf(view) {
    return VIEW_LEVEL[view] || "context";
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

  // layoutView draws whatever view a resolved state names.
  function layoutView(data, state) {
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
      case "diagram":
        return layoutDiagram(data, findDiagram(data, state.of).diagram);
    }
    return layoutContext(data);
  }

  // -------------------------------------------------------------- viewport
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
    var nodes = {};
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
    if (CHANGE_WORDS[n.change]) parts.push(CHANGE_WORDS[n.change] + " by a proposal");
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
      var space = target.indexOf(" ");
      var method = space > 0 && /^[A-Z]+$/.test(target.slice(0, space)) ? target.slice(0, space) : "";
      var pattern = method ? target.slice(space + 1) : target;
      data.facts.routes.forEach(function (r) {
        if (!pkg && r.pattern === pattern && (!method || r.method === method)) pkg = r.package;
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
