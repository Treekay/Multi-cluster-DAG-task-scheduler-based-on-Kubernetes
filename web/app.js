const state = {
  workflow: null,
  clusters: [],
  examples: [],
  simulation: null,
  stepIndex: 0,
  timer: null,
  dagView: {
    x: 0,
    y: 0,
    width: 1000,
    height: 560,
    contentWidth: 1000,
    contentHeight: 560,
    key: "",
    dragging: false,
    dragStart: null,
  },
};

const els = {
  workflowName: document.querySelector("#workflowName"),
  dagCanvas: document.querySelector("#dagCanvas"),
  clusters: document.querySelector("#clusters"),
  events: document.querySelector("#events"),
  eventCount: document.querySelector("#eventCount"),
  stepLabel: document.querySelector("#stepLabel"),
  workflowEditor: document.querySelector("#workflowEditor"),
  workflowSelect: document.querySelector("#workflowSelect"),
  zoomOutBtn: document.querySelector("#zoomOutBtn"),
  zoomResetBtn: document.querySelector("#zoomResetBtn"),
  zoomInBtn: document.querySelector("#zoomInBtn"),
  runBtn: document.querySelector("#runBtn"),
  runKubernetesBtn: document.querySelector("#runKubernetesBtn"),
  resetBtn: document.querySelector("#resetBtn"),
  simulateJsonBtn: document.querySelector("#simulateJsonBtn"),
};

els.runBtn.addEventListener("click", runSimulation);
els.runKubernetesBtn.addEventListener("click", runKubernetes);
els.resetBtn.addEventListener("click", resetView);
els.simulateJsonBtn.addEventListener("click", applyEditor);
els.workflowSelect.addEventListener("change", selectWorkflowExample);
els.zoomOutBtn.addEventListener("click", () => zoomDag(1.2));
els.zoomInBtn.addEventListener("click", () => zoomDag(0.8));
els.zoomResetBtn.addEventListener("click", resetDagViewport);
els.dagCanvas.addEventListener("wheel", handleDagWheel, { passive: false });
els.dagCanvas.addEventListener("pointerdown", startDagPan);
window.addEventListener("pointermove", moveDagPan);
window.addEventListener("pointerup", stopDagPan);
els.dagCanvas.addEventListener("dblclick", resetDagViewport);

loadDefaults();

async function loadDefaults() {
  const response = await fetch("/api/default");
  const payload = await response.json();
  if (!response.ok) {
    throw new Error(payload.error || "failed to load defaults");
  }

  state.workflow = payload.workflow;
  state.clusters = payload.clusters;
  state.examples = payload.examples || [];
  renderWorkflowOptions();
  els.workflowEditor.value = JSON.stringify(state.workflow, null, 2);
  resetView();
}

async function runSimulation() {
  stopTimer();
  clearEvents();
  setBusy(true);
  const response = await fetch("/api/simulate", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ workflow: state.workflow, clusters: state.clusters }),
  });
  const payload = await response.json();
  if (!response.ok) {
    appendError(payload.error || "simulation failed");
    setBusy(false);
    return;
  }

  state.simulation = payload;
  state.stepIndex = 0;
  renderStep(payload.steps[0]);
  state.timer = window.setInterval(() => {
    state.stepIndex += 1;
    if (state.stepIndex >= state.simulation.steps.length) {
      stopTimer();
      setBusy(false);
      return;
    }
    renderStep(state.simulation.steps[state.stepIndex]);
  }, 900);
}

async function runKubernetes() {
  stopTimer();
  clearEvents();
  setBusy(true);
  appendEvent({
    index: 0,
    type: "scheduled",
    message: "Submitting workflow to Kubernetes. This may take a while...",
  });

  const response = await fetch("/api/kubernetes/run", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ workflow: state.workflow, clusters: state.clusters }),
  });
  const payload = await response.json();
  if (!response.ok && !payload.steps) {
    appendError(payload.error || "Kubernetes execution failed");
    setBusy(false);
    return;
  }

  state.simulation = payload;
  state.stepIndex = 0;
  clearEvents();
  if (payload.steps?.length) {
    renderStep(payload.steps[0]);
    state.timer = window.setInterval(() => {
      state.stepIndex += 1;
      if (state.stepIndex >= state.simulation.steps.length) {
        stopTimer();
        setBusy(false);
        if (payload.error) appendError(payload.error);
        return;
      }
      renderStep(state.simulation.steps[state.stepIndex]);
    }, 650);
    return;
  }

  setBusy(false);
  if (payload.error) appendError(payload.error);
}

function applyEditor() {
  try {
    state.workflow = JSON.parse(els.workflowEditor.value);
    els.workflowSelect.value = "";
    resetView();
  } catch (error) {
    appendError(`Invalid workflow JSON: ${error.message}`);
  }
}

function selectWorkflowExample() {
  const selected = state.examples.find((example) => example.path === els.workflowSelect.value);
  if (!selected) return;
  state.workflow = structuredClone(selected.workflow);
  els.workflowEditor.value = JSON.stringify(state.workflow, null, 2);
  resetView();
}

function renderWorkflowOptions() {
  els.workflowSelect.innerHTML = [
    '<option value="">Custom JSON</option>',
    ...state.examples.map((example) => `<option value="${escapeHtml(example.path)}">${escapeHtml(example.name)}</option>`),
  ].join("");
  const active = state.examples.find((example) => example.workflow.name === state.workflow.name);
  if (active) els.workflowSelect.value = active.path;
}

function resetView() {
  stopTimer();
  clearEvents();
  setBusy(false);
  state.simulation = null;
  state.stepIndex = 0;
  const tasks = state.workflow.tasks.map((task) => ({
    name: task.name,
    status: "Pending",
    dependsOn: task.dependsOn || [],
    cpu: task.resources?.cpu || 0,
    memoryMiB: task.resources?.memoryMiB || 0,
  }));
  const clusters = state.clusters.map((cluster) => ({
    name: cluster.name,
    context: cluster.context,
    namespace: cluster.namespace,
    cpuCapacity: cluster.capacity.cpu,
    cpuUsed: 0,
    memoryCapacity: cluster.capacity.memoryMiB,
    memoryUsed: 0,
  }));
  render({
    index: 0,
    type: "reset",
    message: "Ready to simulate.",
    tasks,
    clusters,
  });
}

function renderStep(step) {
  render(step);
  appendEvent(step);
}

function render(step) {
  els.workflowName.textContent = state.workflow.name;
  els.stepLabel.textContent = `Step ${step.index}`;
  renderDag(step.tasks);
  renderClusters(step.clusters);
}

function renderDag(tasks) {
  const layout = computeLayout(tasks);
  syncDagViewport(layout, tasks);
  const edges = [];
  for (const task of tasks) {
    for (const dependency of task.dependsOn || []) {
      edges.push({ from: dependency, to: task.name });
    }
  }

  const edgeMarkup = edges
    .map((edge) => {
      const from = layout.positions.get(edge.from);
      const to = layout.positions.get(edge.to);
      if (!from || !to) return "";
      const startX = from.x + layout.nodeWidth;
      const startY = from.y + layout.nodeHeight / 2;
      const endX = to.x;
      const endY = to.y + layout.nodeHeight / 2;
      const midX = (startX + endX) / 2;
      return `<path class="edge" d="M ${startX} ${startY} C ${midX} ${startY}, ${midX} ${endY}, ${endX} ${endY}" />`;
    })
    .join("");

  const nodeMarkup = tasks
    .map((task) => {
      const pos = layout.positions.get(task.name);
      const cluster = task.cluster ? ` on ${task.cluster}` : "";
      return `
        <g class="node ${task.status}" transform="translate(${pos.x}, ${pos.y})">
          <rect width="${layout.nodeWidth}" height="${layout.nodeHeight}" rx="8"></rect>
          <text class="node-title" x="18" y="30">
            <title>${escapeHtml(task.name)}</title>
            ${escapeHtml(compactTaskName(task.name))}
          </text>
          <text class="node-meta" x="18" y="54">${task.cpu} CPU · ${task.memoryMiB} MiB</text>
          <text class="node-status ${task.status}" x="18" y="74">${task.status}${escapeHtml(cluster)}</text>
        </g>`;
    })
    .join("");

  els.dagCanvas.innerHTML = `<rect class="dag-background" x="0" y="0" width="${layout.width}" height="${layout.height}"></rect>${edgeMarkup}${nodeMarkup}`;
  applyDagViewport();
}

function computeLayout(tasks) {
  const nodeWidth = 236;
  const nodeHeight = 88;
  const marginX = 80;
  const marginY = 64;
  const columnGap = 90;
  const rowGap = 72;
  const byName = new Map(tasks.map((task) => [task.name, task]));
  const depthMemo = new Map();
  function depth(name) {
    if (depthMemo.has(name)) return depthMemo.get(name);
    const task = byName.get(name);
    const deps = task?.dependsOn || [];
    const value = deps.length === 0 ? 0 : Math.max(...deps.map(depth)) + 1;
    depthMemo.set(name, value);
    return value;
  }

  const layers = new Map();
  for (const task of tasks) {
    const taskDepth = depth(task.name);
    if (!layers.has(taskDepth)) layers.set(taskDepth, []);
    layers.get(taskDepth).push(task);
  }

  const positions = new Map();
  const orderedLayers = Array.from(layers.entries()).sort(([a], [b]) => a - b);
  const maxLayerSize = Math.max(...orderedLayers.map(([, layerTasks]) => layerTasks.length), 1);
  const width = Math.max(1000, marginX * 2 + orderedLayers.length * nodeWidth + Math.max(0, orderedLayers.length - 1) * columnGap);
  const height = Math.max(560, marginY * 2 + maxLayerSize * nodeHeight + Math.max(0, maxLayerSize - 1) * rowGap);

  for (const [layer, layerTasks] of orderedLayers) {
    const layerHeight = layerTasks.length * nodeHeight + Math.max(0, layerTasks.length - 1) * rowGap;
    const startY = marginY + (height - marginY * 2 - layerHeight) / 2;
    layerTasks.forEach((task, index) => {
      positions.set(task.name, {
        x: marginX + layer * (nodeWidth + columnGap),
        y: startY + index * (nodeHeight + rowGap),
      });
    });
  }

  return { positions, width, height, nodeWidth, nodeHeight };
}

function compactTaskName(name) {
  if (name.length <= 18) return name;
  return `${name.slice(0, 16)}...`;
}

function syncDagViewport(layout, tasks) {
  const key = `${layout.width}x${layout.height}:${tasks.map((task) => task.name).join("|")}`;
  state.dagView.contentWidth = layout.width;
  state.dagView.contentHeight = layout.height;
  if (state.dagView.key !== key) {
    state.dagView.key = key;
    resetDagViewport();
  }
}

function resetDagViewport() {
  const panel = els.dagCanvas.getBoundingClientRect();
  const panelRatio = panel.width / Math.max(panel.height, 1);
  const contentRatio = state.dagView.contentWidth / Math.max(state.dagView.contentHeight, 1);

  if (panelRatio > contentRatio) {
    state.dagView.height = state.dagView.contentHeight;
    state.dagView.width = state.dagView.height * panelRatio;
  } else {
    state.dagView.width = state.dagView.contentWidth;
    state.dagView.height = state.dagView.width / Math.max(panelRatio, 0.1);
  }

  state.dagView.x = (state.dagView.contentWidth - state.dagView.width) / 2;
  state.dagView.y = (state.dagView.contentHeight - state.dagView.height) / 2;
  applyDagViewport();
}

function applyDagViewport() {
  clampDagViewport();
  els.dagCanvas.setAttribute(
    "viewBox",
    `${state.dagView.x} ${state.dagView.y} ${state.dagView.width} ${state.dagView.height}`,
  );
}

function clampDagViewport() {
  const maxWidth = state.dagView.contentWidth * 1.25;
  const maxHeight = state.dagView.contentHeight * 1.25;
  const minWidth = Math.max(320, state.dagView.contentWidth * 0.2);
  const minHeight = Math.max(220, state.dagView.contentHeight * 0.2);
  state.dagView.width = Math.min(maxWidth, Math.max(minWidth, state.dagView.width));
  state.dagView.height = Math.min(maxHeight, Math.max(minHeight, state.dagView.height));

  if (state.dagView.width >= state.dagView.contentWidth) {
    state.dagView.x = (state.dagView.contentWidth - state.dagView.width) / 2;
  } else {
    state.dagView.x = Math.min(Math.max(0, state.dagView.x), state.dagView.contentWidth - state.dagView.width);
  }
  if (state.dagView.height >= state.dagView.contentHeight) {
    state.dagView.y = (state.dagView.contentHeight - state.dagView.height) / 2;
  } else {
    state.dagView.y = Math.min(Math.max(0, state.dagView.y), state.dagView.contentHeight - state.dagView.height);
  }
}

function zoomDag(scale, clientPoint) {
  const rect = els.dagCanvas.getBoundingClientRect();
  const point = clientPoint || { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 };
  const localX = (point.x - rect.left) / Math.max(rect.width, 1);
  const localY = (point.y - rect.top) / Math.max(rect.height, 1);
  const focusX = state.dagView.x + localX * state.dagView.width;
  const focusY = state.dagView.y + localY * state.dagView.height;
  const nextWidth = state.dagView.width * scale;
  const nextHeight = state.dagView.height * scale;
  state.dagView.x = focusX - localX * nextWidth;
  state.dagView.y = focusY - localY * nextHeight;
  state.dagView.width = nextWidth;
  state.dagView.height = nextHeight;
  applyDagViewport();
}

function handleDagWheel(event) {
  event.preventDefault();
  zoomDag(event.deltaY > 0 ? 1.12 : 0.88, { x: event.clientX, y: event.clientY });
}

function startDagPan(event) {
  if (event.button !== 0 || event.target.closest(".node")) return;
  state.dagView.dragging = true;
  state.dagView.dragStart = {
    clientX: event.clientX,
    clientY: event.clientY,
    x: state.dagView.x,
    y: state.dagView.y,
  };
  els.dagCanvas.setPointerCapture(event.pointerId);
  els.dagCanvas.classList.add("is-panning");
}

function moveDagPan(event) {
  if (!state.dagView.dragging || !state.dagView.dragStart) return;
  const rect = els.dagCanvas.getBoundingClientRect();
  const dx = ((event.clientX - state.dagView.dragStart.clientX) / Math.max(rect.width, 1)) * state.dagView.width;
  const dy = ((event.clientY - state.dagView.dragStart.clientY) / Math.max(rect.height, 1)) * state.dagView.height;
  state.dagView.x = state.dagView.dragStart.x - dx;
  state.dagView.y = state.dagView.dragStart.y - dy;
  applyDagViewport();
}

function stopDagPan() {
  state.dagView.dragging = false;
  state.dagView.dragStart = null;
  els.dagCanvas.classList.remove("is-panning");
}

function renderClusters(clusters) {
  els.clusters.innerHTML = clusters
    .map((cluster) => {
      const cpuPercent = percent(cluster.cpuUsed, cluster.cpuCapacity);
      const memoryPercent = percent(cluster.memoryUsed, cluster.memoryCapacity);
      return `
        <article class="cluster">
          <h3>${escapeHtml(cluster.name)}</h3>
          <p>${escapeHtml(cluster.context || "local simulation")}</p>
          <div class="meter">
            <div class="meter-label"><span>CPU</span><span>${cluster.cpuUsed}/${cluster.cpuCapacity}</span></div>
            <div class="bar"><span style="width:${cpuPercent}%"></span></div>
          </div>
          <div class="meter">
            <div class="meter-label"><span>Memory</span><span>${cluster.memoryUsed}/${cluster.memoryCapacity} MiB</span></div>
            <div class="bar"><span style="width:${memoryPercent}%"></span></div>
          </div>
        </article>`;
    })
    .join("");
}

function appendEvent(step) {
  const item = document.createElement("li");
  item.className = step.type;
  item.textContent = `${step.index}. ${step.message}`;
  els.events.prepend(item);
  els.eventCount.textContent = `${els.events.children.length} events`;
}

function appendError(message) {
  const item = document.createElement("li");
  item.className = "failed";
  item.textContent = message;
  els.events.prepend(item);
  els.eventCount.textContent = `${els.events.children.length} events`;
}

function clearEvents() {
  els.events.innerHTML = "";
  els.eventCount.textContent = "0 events";
}

function setBusy(isBusy) {
  els.runBtn.disabled = isBusy;
  els.runKubernetesBtn.disabled = isBusy;
  els.simulateJsonBtn.disabled = isBusy;
}

function stopTimer() {
  if (state.timer) {
    window.clearInterval(state.timer);
    state.timer = null;
  }
}

function percent(used, capacity) {
  if (!capacity) return 0;
  return Math.min(100, Math.round((used / capacity) * 100));
}

function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}
