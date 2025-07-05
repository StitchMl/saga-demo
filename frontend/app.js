// ———DOM elements———
const tabOrch = document.getElementById('tab-orch');
const tabChoreo = document.getElementById('tab-choreo');
const orchPanel = document.getElementById('orch-panel');
const choreoPanel = document.getElementById('choreo-panel');

const orchestratorForm = document.getElementById('orch-form');
const orchestratorOrderId = document.getElementById('orch-order-id');
const orchestratorAmount = document.getElementById('orch-amount');
const orchestratorFb = document.getElementById('orch-feedback');

const choreoForm = document.getElementById('choreo-form');
const choreoOrderId = document.getElementById('choreo-order-id');
const choreoAmount = document.getElementById('choreo-amount');
const choreoFb = document.getElementById('choreo-feedback');

const themeToggle = document.getElementById('theme-toggle');

// ———Tab switching———
tabOrch.addEventListener('click', () => {
    tabOrch.classList.add('border-purple-600', 'text-purple-600');
    tabChoreo.classList.remove('border-purple-600', 'text-purple-600');
    tabChoreo.classList.add('text-gray-500');
    orchPanel.classList.remove('hidden');
    choreoPanel.classList.add('hidden');
});
tabChoreo.addEventListener('click', () => {
    tabChoreo.classList.add('border-purple-600', 'text-purple-600');
    tabOrch.classList.remove('border-purple-600', 'text-purple-600');
    tabOrch.classList.add('text-gray-500');
    choreoPanel.classList.remove('hidden');
    orchPanel.classList.add('hidden');
});

// ———Dark mode toggle———
themeToggle.addEventListener('click', () => {
    document.documentElement.classList.toggle('dark');
});

// ———Helper fetches con timeout———
async function fetchWithTimeout(url, opts = {}, timeout = 7000) {
    const ctrl = new AbortController();
    const id = setTimeout(() => ctrl.abort(), timeout);
    const res = await fetch(url, { ...opts, signal: ctrl.signal });
    clearTimeout(id);
    return res;
}

// ———Orchestrated flow———
orchestratorForm.addEventListener('submit', async e => {
    e.preventDefault();
    const id = orchestratorOrderId.value.trim();
    const amount = parseFloat(orchestratorAmount.value);
    orchestratorFb.textContent = 'Start of the orchestrated saga...';

    try {
        const res = await fetchWithTimeout('/saga', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ id, amount })
        }, 10000); // 10-second timeout for the orchestrator

        const txt = await res.text();
        orchestratorFb.textContent = `Status ${res.status}: ${txt}`;
    } catch (err) {
        orchestratorFb.textContent = `❌ Error: ${err.message}`;
        if (err.name === 'AbortError') {
            orchestratorFb.textContent += ' (Request timeout)';
        }
    }
});

// ———Choreographic flow———
choreoForm.addEventListener('submit', async e => {
    e.preventDefault();
    const id = choreoOrderId.value.trim();
    const amount = parseFloat(choreoAmount.value);
    choreoFb.textContent = 'Starting the choreographic flow (sending order)...';

    try {
        // It also applies fetchWithTimeout here, with an appropriate timeout for the order service.
        const res = await fetchWithTimeout('/orders', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ OrderID: id, Amount: amount })
        }, 7000); // 7-second timeout for the order service

        if (res.status !== 201) {
            const errorText = await res.text();
            choreoFb.textContent = `❌ Order Service returned ${res.status}: ${errorText || 'unknown error'}`;
            return;
        }

        // Improved feedback for asynchronous choreographic flow.
        choreoFb.textContent = `✔️ Event OrderCreated successfully sent for Order ID: ${id}.`;
        choreoFb.textContent += `\nThe payment and shipping process will now take place via Event Bus.`;
        choreoFb.textContent += `\nSince it is a choreographed asynchronous flow, the final state must be verified via the logs of the backend microservices (Payment, Shipping).`;

    } catch (err) {
        choreoFb.textContent = `❌ Error: ${err.message}`;
        if (err.name === 'AbortError') {
            choreoFb.textContent += ' (Request timeout)';
        }
    }
});