const state = {
  workflow: null,
  clusters: [],
  simulation: null,
  stepIndex: 0,
  timer: null,
};

const els = {
  workflowName: document.querySelector("#workflowName"),
  dagCanvas: document.querySelector("#dagCanvas"),
  clusters: document.querySelector("#clusters"),
  events: document.querySelector("#events"),
  eventCount: document.querySelector("#eventCount"),
  stepLabel: document.querySelector("#stepLabel"),
  workflowEditor: document.querySelector("#workflowEditor"),
  runBtn: document.querySelector("#runBtn"),
  runKubernetesBtn: document.querySelector("#runKubernetesBtn"),
  resetBtn: document.querySelector("#resetBtn"),
  simulateJsonBtn: document.querySelector("#simulateJsonBtn"),
};

els.runBtn.addEventListener("click", runSimulation);
els.runKubernetesBtn.addEventListener("click", runKubernetes);
els.resetBtn.addEventListener("click", resetView);
els.simulateJsonBtn.addEventListener("click", applyEditor);

loadDefaults();

async function loadDefaults() {
  const response = await fetch("/api/default");
  const payload = await response.json();
  if (!response.ok) {
    throw new Error(payload.error || "failed to load defaults");
  }

  state.workflow = payload.workflow;
  state.clusters = payload.clusters;
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
    resetView();
  } catch (error) {
    appendError(`Invalid workflow JSON: ${error.message}`);
  }
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
      const startX = from.x + 92;
      const startY = from.y + 44;
      const endX = to.x - 92;
      const endY = to.y + 44;
      const midX = (startX + endX) / 2;
      return `<path class="edge" d="M ${startX} ${startY} C ${midX} ${startY}, ${midX} ${endY}, ${endX} ${endY}" />`;
    })
    .join("");

  const nodeMarkup = tasks
    .map((task) => {
      const pos = layout.positions.get(task.name);
      const cluster = task.cluster ? ` on ${task.cluster}` : "";
      return `
        <g class="node ${task.status}" transform="translate(${pos.x - 92}, ${pos.y})">
          <rect width="184" height="88" rx="8"></rect>
          <text class="node-title" x="18" y="30">${escapeHtml(task.name)}</text>
          <text class="node-meta" x="18" y="54">${task.cpu} CPU · ${task.memoryMiB} MiB</text>
          <text class="node-status ${task.status}" x="18" y="74">${task.status}${escapeHtml(cluster)}</text>
        </g>`;
    })
    .join("");

  els.dagCanvas.innerHTML = `${edgeMarkup}${nodeMarkup}`;
}

function computeLayout(tasks) {
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
  const maxDepth = Math.max(...Array.from(layers.keys()), 0);
  const xGap = maxDepth === 0 ? 0 : 760 / maxDepth;
  for (const [layer, layerTasks] of layers.entries()) {
    const yGap = 420 / Math.max(layerTasks.length, 1);
    layerTasks.forEach((task, index) => {
      positions.set(task.name, {
        x: 120 + layer * xGap,
        y: 78 + index * yGap,
      });
    });
  }

  return { positions };
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
