// Waggle DAG Visualization — app.js
// Uses D3.js v7 for force-directed graph layout.

'use strict';

// ---- State ------------------------------------------------------------------

const state = {
  nodes: [],       // [{ id, name, status, metrics }]
  edges: [],       // [{ from, to, dataSize, latencyMs }]
  selected: null,  // selected node id
  events: [],      // recent events for the log (capped at 200)
};

const MAX_LOG_ENTRIES = 200;

// Node status constants matching the backend.
const STATUS = { WAITING: 'waiting', RUNNING: 'running', SUCCESS: 'success', ERROR: 'error' };

// ---- DOM references ---------------------------------------------------------

const dagSvg        = document.getElementById('dag-svg');
const eventLog      = document.getElementById('event-log');
const agentDetail   = document.getElementById('agent-detail');
const noSelection   = document.getElementById('no-selection');
const connStatus    = document.getElementById('connection-status');
const connLabel     = document.getElementById('connection-label');

// ---- D3 setup ---------------------------------------------------------------

const svgEl = d3.select('#dag-svg');
const width  = () => dagSvg.clientWidth;
const height = () => dagSvg.clientHeight;

// Root group for pan/zoom.
const root = svgEl.append('g').attr('class', 'root');

// Arrow marker for edges.
svgEl.append('defs').append('marker')
  .attr('id', 'arrow')
  .attr('viewBox', '0 -5 10 10')
  .attr('refX', 28)
  .attr('refY', 0)
  .attr('markerWidth', 6)
  .attr('markerHeight', 6)
  .attr('orient', 'auto')
  .append('path')
  .attr('d', 'M0,-5L10,0L0,5')
  .attr('fill', '#4a5568');

const edgeGroup = root.append('g').attr('class', 'edges');
const nodeGroup = root.append('g').attr('class', 'nodes');

// Zoom behaviour.
const zoom = d3.zoom()
  .scaleExtent([0.1, 4])
  .on('zoom', (event) => root.attr('transform', event.transform));
svgEl.call(zoom);

// Force simulation.
let simulation = null;

// ---- Rendering --------------------------------------------------------------

function renderDAG() {
  // Stop any running simulation.
  if (simulation) simulation.stop();

  const nodes = state.nodes.map(n => ({ ...n }));
  const edges = state.edges.map(e => ({ ...e }));

  // Map from node id to node object (needed by D3 force).
  const nodeById = new Map(nodes.map(n => [n.id, n]));
  const links = edges
    .filter(e => nodeById.has(e.from) && nodeById.has(e.to))
    .map(e => ({ source: e.from, target: e.to, meta: e }));

  // ---- Edges ----
  const edgeSel = edgeGroup.selectAll('.edge')
    .data(links, d => `${d.source}-${d.target}`)
    .join(
      enter => {
        const g = enter.append('g').attr('class', 'edge');
        g.append('path').attr('marker-end', 'url(#arrow)');
        g.append('text').attr('class', 'edge-label').attr('text-anchor', 'middle');
        return g;
      }
    );

  // ---- Nodes ----
  const NODE_W = 140, NODE_H = 44;

  const nodeSel = nodeGroup.selectAll('.node')
    .data(nodes, d => d.id)
    .join(
      enter => {
        const g = enter.append('g')
          .attr('class', d => `node ${d.status || STATUS.WAITING}`)
          .call(d3.drag()
            .on('start', dragStarted)
            .on('drag',  dragged)
            .on('end',   dragEnded))
          .on('click', (event, d) => {
            event.stopPropagation();
            selectNode(d.id);
          });

        g.append('rect')
          .attr('width', NODE_W)
          .attr('height', NODE_H)
          .attr('x', -NODE_W / 2)
          .attr('y', -NODE_H / 2);

        g.append('text')
          .attr('class', 'node-name')
          .attr('text-anchor', 'middle')
          .attr('dy', '-4px');

        g.append('text')
          .attr('class', 'node-meta')
          .attr('text-anchor', 'middle')
          .attr('dy', '12px');

        return g;
      }
    );

  // Update node class and text.
  nodeSel.attr('class', d => `node ${d.status || STATUS.WAITING}${d.id === state.selected ? ' selected' : ''}`)
    .select('.node-name').text(d => d.name);

  nodeSel.select('.node-meta').text(d => {
    const m = d.metrics;
    if (!m || m.total_runs === 0) return '';
    return `${m.avg_duration_ms}ms · ${(m.error_rate * 100).toFixed(0)}% err`;
  });

  // ---- Simulation ----
  simulation = d3.forceSimulation(nodes)
    .force('link', d3.forceLink(links).id(d => d.id).distance(180).strength(1))
    .force('charge', d3.forceManyBody().strength(-400))
    .force('center', d3.forceCenter(width() / 2, height() / 2))
    .force('collision', d3.forceCollide(80))
    .on('tick', () => {
      edgeSel.select('path').attr('d', d => {
        const src = nodeById.get(d.source.id || d.source);
        const tgt = nodeById.get(d.target.id || d.target);
        if (!src || !tgt) return '';
        return `M${src.x},${src.y}L${tgt.x},${tgt.y}`;
      });
      edgeSel.select('.edge-label').attr('transform', d => {
        const src = nodeById.get(d.source.id || d.source);
        const tgt = nodeById.get(d.target.id || d.target);
        if (!src || !tgt) return '';
        return `translate(${(src.x + tgt.x) / 2},${(src.y + tgt.y) / 2})`;
      }).text(d => {
        if (!d.meta) return '';
        const parts = [];
        if (d.meta.latency_ms) parts.push(`${d.meta.latency_ms}ms`);
        if (d.meta.data_size)  parts.push(formatBytes(d.meta.data_size));
        return parts.join(' · ');
      });

      nodeSel.attr('transform', d => `translate(${d.x},${d.y})`);
    });
}

// ---- Drag helpers -----------------------------------------------------------

function dragStarted(event, d) {
  if (!event.active) simulation.alphaTarget(0.3).restart();
  d.fx = d.x; d.fy = d.y;
}
function dragged(event, d) { d.fx = event.x; d.fy = event.y; }
function dragEnded(event, d) {
  if (!event.active) simulation.alphaTarget(0);
  d.fx = null; d.fy = null;
}

// ---- Selection & detail panel -----------------------------------------------

svgEl.on('click', () => selectNode(null));

function selectNode(id) {
  state.selected = id;
  if (!id) {
    agentDetail.classList.add('hidden');
    noSelection.classList.remove('hidden');
    nodeGroup.selectAll('.node').attr('class', d => `node ${d.status || STATUS.WAITING}`);
    return;
  }
  const node = state.nodes.find(n => n.id === id);
  if (!node) return;

  agentDetail.classList.remove('hidden');
  noSelection.classList.add('hidden');

  document.getElementById('detail-name').textContent = node.name;
  document.getElementById('detail-status').textContent = node.status || STATUS.WAITING;
  const m = node.metrics || {};
  document.getElementById('detail-runs').textContent       = m.total_runs   ?? '—';
  document.getElementById('detail-avg-dur').textContent    = m.avg_duration_ms != null ? `${m.avg_duration_ms}ms` : '—';
  document.getElementById('detail-error-rate').textContent = m.error_rate    != null ? `${(m.error_rate * 100).toFixed(1)}%` : '—';
  document.getElementById('detail-input-size').textContent  = m.total_input_bytes  != null ? formatBytes(m.total_input_bytes)  : '—';
  document.getElementById('detail-output-size').textContent = m.total_output_bytes != null ? formatBytes(m.total_output_bytes) : '—';

  // Highlight selected node.
  nodeGroup.selectAll('.node').attr('class', d =>
    `node ${d.status || STATUS.WAITING}${d.id === id ? ' selected' : ''}`
  );
}

// ---- Event log --------------------------------------------------------------

function appendLogEntry(event) {
  const el = document.createElement('div');
  const ts = new Date(event.timestamp).toLocaleTimeString();
  el.className = `log-entry ${eventTypeClass(event.type)}`;
  el.innerHTML = `<span class="ts">${ts}</span>${event.type} — ${event.agent_name || ''}`;
  el.title = JSON.stringify(event, null, 2);
  eventLog.prepend(el);

  // Cap log entries.
  while (eventLog.childElementCount > MAX_LOG_ENTRIES) {
    eventLog.removeChild(eventLog.lastChild);
  }
}

function eventTypeClass(type) {
  if (type.includes('start'))   return 'start';
  if (type.includes('end'))     return 'end';
  if (type.includes('error'))   return 'error';
  if (type.includes('flow'))    return 'flow';
  return '';
}

document.getElementById('btn-clear-log').addEventListener('click', () => {
  eventLog.innerHTML = '';
});

// ---- API calls --------------------------------------------------------------

async function fetchDAG() {
  try {
    const resp = await fetch('/api/dag');
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    const data = await resp.json();
    state.nodes = data.nodes || [];
    state.edges = data.edges || [];
    renderDAG();
  } catch (err) {
    console.error('fetchDAG error:', err);
  }
}

async function fetchMetrics() {
  try {
    const resp = await fetch('/api/metrics');
    if (!resp.ok) return;
    const data = await resp.json();
    // Merge metrics into node state.
    const metricsById = {};
    (data.agents || []).forEach(m => { metricsById[m.agent_name] = m; });
    state.nodes = state.nodes.map(n => ({ ...n, metrics: metricsById[n.id] || n.metrics }));
    renderDAG();
  } catch (err) {
    console.error('fetchMetrics error:', err);
  }
}

// ---- SSE connection ---------------------------------------------------------

function connectSSE() {
  const es = new EventSource('/api/events');

  es.onopen = () => {
    connStatus.className = 'status-dot connected';
    connLabel.textContent = 'Connected';
  };

  es.onerror = () => {
    connStatus.className = 'status-dot error';
    connLabel.textContent = 'Reconnecting...';
    setTimeout(connectSSE, 3000);
    es.close();
  };

  es.onmessage = (e) => {
    try {
      const event = JSON.parse(e.data);
      appendLogEntry(event);

      // Update node status based on event type.
      const node = state.nodes.find(n => n.id === event.agent_name || n.name === event.agent_name);
      if (node) {
        if (event.type === 'agent.start')  node.status = STATUS.RUNNING;
        if (event.type === 'agent.end')    node.status = STATUS.SUCCESS;
        if (event.type === 'agent.error')  node.status = STATUS.ERROR;
        renderDAG();
        if (state.selected === node.id) selectNode(node.id);
      }
    } catch (err) {
      console.error('SSE parse error:', err);
    }
  };
}

// ---- Controls ---------------------------------------------------------------

document.getElementById('btn-refresh').addEventListener('click', () => {
  fetchDAG();
  fetchMetrics();
});

document.getElementById('btn-fit').addEventListener('click', () => {
  svgEl.transition().duration(400).call(
    zoom.transform,
    d3.zoomIdentity.translate(width() / 2, height() / 2).scale(0.9)
  );
});

window.addEventListener('resize', () => {
  if (simulation) simulation.force('center', d3.forceCenter(width() / 2, height() / 2)).alpha(0.1).restart();
});

// ---- Utilities --------------------------------------------------------------

function formatBytes(bytes) {
  if (bytes < 1024) return `${bytes}B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)}MB`;
}

// ---- Bootstrap --------------------------------------------------------------

fetchDAG();
connectSSE();

// Refresh metrics every 5 seconds.
setInterval(fetchMetrics, 5000);
