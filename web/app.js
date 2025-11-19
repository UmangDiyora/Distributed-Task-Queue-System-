// API Base URL
const API_BASE = window.location.origin;

// State
let jobs = [];
let queues = [];
let schedules = [];
let refreshInterval;

// Initialize dashboard
document.addEventListener('DOMContentLoaded', () => {
    initializeJobTypePayloads();
    setupEventListeners();
    loadQueues();
    loadJobs();
    loadSchedules();
    loadMetrics();

    // Auto-refresh every 5 seconds
    refreshInterval = setInterval(() => {
        loadJobs();
        loadMetrics();
        loadQueues();
    }, 5000);
});

// Job type payload templates
const jobTypePayloads = {
    echo: { message: "Hello, World!" },
    math: { operation: "add", a: 10, b: 5 },
    email: { to: "user@example.com", subject: "Test", body: "Test message" },
    report: { type: "daily" }
};

function initializeJobTypePayloads() {
    const jobTypeSelect = document.getElementById('jobType');
    const payloadTextarea = document.getElementById('payload');

    jobTypeSelect.addEventListener('change', (e) => {
        const payload = jobTypePayloads[e.target.value] || {};
        payloadTextarea.value = JSON.stringify(payload, null, 2);
    });

    // Set initial payload
    payloadTextarea.value = JSON.stringify(jobTypePayloads.echo, null, 2);
}

function setupEventListeners() {
    document.getElementById('createJobForm').addEventListener('submit', createJob);
    document.getElementById('filterStatus').addEventListener('change', loadJobs);
    document.getElementById('filterQueue').addEventListener('change', loadJobs);
}

// API Functions
async function apiCall(endpoint, options = {}) {
    try {
        const response = await fetch(`${API_BASE}${endpoint}`, {
            headers: {
                'Content-Type': 'application/json',
                ...options.headers
            },
            ...options
        });

        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }

        return await response.json();
    } catch (error) {
        console.error('API call failed:', error);
        showNotification('API call failed: ' + error.message, 'error');
        throw error;
    }
}

async function createJob(e) {
    e.preventDefault();

    const jobType = document.getElementById('jobType').value;
    const queue = document.getElementById('queue').value;
    const priority = parseInt(document.getElementById('priority').value);
    const payloadText = document.getElementById('payload').value;

    let payload;
    try {
        payload = JSON.parse(payloadText);
    } catch (error) {
        showNotification('Invalid JSON payload', 'error');
        return;
    }

    const job = {
        id: `job_${Date.now()}`,
        type: jobType,
        queue: queue,
        priority: priority,
        payload: payload,
        status: 'pending',
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        scheduled_at: new Date().toISOString()
    };

    try {
        await apiCall('/api/v1/jobs', {
            method: 'POST',
            body: JSON.stringify(job)
        });

        showNotification('Job created successfully!', 'success');
        loadJobs();
        loadMetrics();
    } catch (error) {
        showNotification('Failed to create job', 'error');
    }
}

async function loadJobs() {
    const status = document.getElementById('filterStatus').value;
    const queue = document.getElementById('filterQueue').value;

    let url = '/api/v1/jobs?limit=20';
    if (status) url += `&status=${status}`;
    if (queue) url += `&queue=${queue}`;

    try {
        const data = await apiCall(url);
        jobs = data.jobs || [];
        renderJobs();
    } catch (error) {
        document.getElementById('jobsList').innerHTML = '<p class="empty-state">Failed to load jobs</p>';
    }
}

async function loadQueues() {
    try {
        const data = await apiCall('/api/v1/queues');
        queues = data.queues || [];
        renderQueues();
        updateQueueSelectors();
    } catch (error) {
        document.getElementById('queuesList').innerHTML = '<p class="empty-state">Failed to load queues</p>';
    }
}

async function loadSchedules() {
    try {
        const data = await apiCall('/api/v1/schedules');
        schedules = data.schedules || [];
        renderSchedules();
    } catch (error) {
        document.getElementById('schedulesList').innerHTML = '<p class="empty-state">Failed to load schedules</p>';
    }
}

async function loadMetrics() {
    try {
        const data = await apiCall('/api/v1/metrics');
        updateMetricsDisplay(data);
    } catch (error) {
        console.error('Failed to load metrics:', error);
    }
}

async function cancelJob(jobId) {
    if (!confirm('Are you sure you want to cancel this job?')) return;

    try {
        await apiCall(`/api/v1/jobs/${jobId}/cancel`, { method: 'POST' });
        showNotification('Job cancelled successfully', 'success');
        loadJobs();
        loadMetrics();
    } catch (error) {
        showNotification('Failed to cancel job', 'error');
    }
}

async function deleteJob(jobId) {
    if (!confirm('Are you sure you want to delete this job?')) return;

    try {
        await apiCall(`/api/v1/jobs/${jobId}`, { method: 'DELETE' });
        showNotification('Job deleted successfully', 'success');
        loadJobs();
        loadMetrics();
    } catch (error) {
        showNotification('Failed to delete job', 'error');
    }
}

async function pauseQueue(queueName) {
    try {
        await apiCall(`/api/v1/queues/${queueName}/pause`, { method: 'POST' });
        showNotification(`Queue "${queueName}" paused`, 'success');
        loadQueues();
    } catch (error) {
        showNotification('Failed to pause queue', 'error');
    }
}

async function resumeQueue(queueName) {
    try {
        await apiCall(`/api/v1/queues/${queueName}/resume`, { method: 'POST' });
        showNotification(`Queue "${queueName}" resumed`, 'success');
        loadQueues();
    } catch (error) {
        showNotification('Failed to resume queue', 'error');
    }
}

async function deleteSchedule(scheduleId) {
    if (!confirm('Are you sure you want to delete this schedule?')) return;

    try {
        await apiCall(`/api/v1/schedules/${scheduleId}`, { method: 'DELETE' });
        showNotification('Schedule deleted successfully', 'success');
        loadSchedules();
    } catch (error) {
        showNotification('Failed to delete schedule', 'error');
    }
}

// Render Functions
function renderJobs() {
    const container = document.getElementById('jobsList');

    if (jobs.length === 0) {
        container.innerHTML = '<div class="empty-state"><h3>No jobs found</h3><p>Create a new job to get started</p></div>';
        return;
    }

    container.innerHTML = jobs.map(job => `
        <div class="job-card status-${job.status}">
            <div class="job-header">
                <span class="job-id">${job.id}</span>
                <span class="job-status ${job.status}">${job.status}</span>
            </div>
            <div class="job-details">
                <div class="job-detail"><strong>Type:</strong> ${job.type}</div>
                <div class="job-detail"><strong>Queue:</strong> ${job.queue}</div>
                <div class="job-detail"><strong>Priority:</strong> ${getPriorityLabel(job.priority)}</div>
                <div class="job-detail"><strong>Retries:</strong> ${job.retry_count || 0}/${job.max_retries || 3}</div>
                <div class="job-detail"><strong>Progress:</strong> ${job.progress || 0}%</div>
                <div class="job-detail"><strong>Created:</strong> ${formatDate(job.created_at)}</div>
            </div>
            ${job.error ? `<div class="job-detail" style="margin-top: 10px; color: #dc3545;"><strong>Error:</strong> ${job.error}</div>` : ''}
            <div class="job-actions">
                ${job.status === 'pending' || job.status === 'running' ?
                    `<button onclick="cancelJob('${job.id}')" class="btn btn-danger">Cancel</button>` : ''}
                <button onclick="deleteJob('${job.id}')" class="btn btn-danger">Delete</button>
            </div>
        </div>
    `).join('');
}

function renderQueues() {
    const container = document.getElementById('queuesList');

    if (queues.length === 0) {
        container.innerHTML = '<div class="empty-state"><p>No queues configured</p></div>';
        return;
    }

    container.innerHTML = queues.map(queue => `
        <div class="queue-card">
            <div class="queue-name">${queue.name}</div>
            <div class="queue-stats">
                <div class="queue-stat"><strong>Max Workers:</strong> ${queue.max_workers || 10}</div>
                <div class="queue-stat"><strong>Max Retries:</strong> ${queue.max_retries || 3}</div>
                <div class="queue-stat"><strong>Priority:</strong> ${getPriorityLabel(queue.priority)}</div>
                <div class="queue-stat"><strong>Weight:</strong> ${queue.weight || 1}</div>
                <div class="queue-stat"><strong>Timeout:</strong> ${queue.timeout || 300}s</div>
                <div class="queue-stat"><strong>Strategy:</strong> ${queue.retry_strategy || 'exponential'}</div>
            </div>
            <div class="queue-actions">
                <button onclick="pauseQueue('${queue.name}')" class="btn btn-danger">Pause</button>
                <button onclick="resumeQueue('${queue.name}')" class="btn btn-success">Resume</button>
            </div>
        </div>
    `).join('');
}

function renderSchedules() {
    const container = document.getElementById('schedulesList');

    if (schedules.length === 0) {
        container.innerHTML = '<div class="empty-state"><p>No schedules configured</p></div>';
        return;
    }

    container.innerHTML = schedules.map(schedule => `
        <div class="schedule-card">
            <div class="schedule-name">${schedule.name || schedule.id}</div>
            <div class="schedule-details"><strong>Job Type:</strong> ${schedule.job_type}</div>
            <div class="schedule-details"><strong>Queue:</strong> ${schedule.queue}</div>
            <div class="schedule-details"><strong>Schedule:</strong> ${schedule.cron_expr || `Every ${formatDuration(schedule.interval)}`}</div>
            <div class="schedule-details"><strong>Next Run:</strong> ${formatDate(schedule.next_run)}</div>
            <div class="schedule-details"><strong>Run Count:</strong> ${schedule.run_count || 0}</div>
            <div class="schedule-details"><strong>Status:</strong> ${schedule.enabled ? '✅ Enabled' : '❌ Disabled'}</div>
            <div class="schedule-actions">
                <button onclick="deleteSchedule('${schedule.id}')" class="btn btn-danger">Delete</button>
            </div>
        </div>
    `).join('');
}

function updateMetricsDisplay(data) {
    const queueStats = data.queue_stats || {};

    let totalJobs = 0;
    let pendingJobs = 0;
    let runningJobs = 0;
    let completedJobs = 0;
    let failedJobs = 0;

    Object.values(queueStats).forEach(stats => {
        pendingJobs += stats.pending_count || 0;
        runningJobs += stats.running_count || 0;
        completedJobs += stats.success_count || 0;
        failedJobs += stats.failed_count || 0;
    });

    totalJobs = pendingJobs + runningJobs + completedJobs + failedJobs;

    document.getElementById('totalJobs').textContent = totalJobs;
    document.getElementById('pendingJobs').textContent = pendingJobs;
    document.getElementById('runningJobs').textContent = runningJobs;
    document.getElementById('completedJobs').textContent = completedJobs;
    document.getElementById('failedJobs').textContent = failedJobs;
}

function updateQueueSelectors() {
    const selectors = [
        document.getElementById('queue'),
        document.getElementById('filterQueue')
    ];

    selectors.forEach(select => {
        const currentValue = select.value;
        const options = queues.map(q =>
            `<option value="${q.name}" ${q.name === currentValue ? 'selected' : ''}>${q.name}</option>`
        ).join('');

        if (select.id === 'filterQueue') {
            select.innerHTML = '<option value="">All Queues</option>' + options;
        } else {
            select.innerHTML = options;
        }
    });
}

// Utility Functions
function formatDate(dateStr) {
    if (!dateStr) return 'N/A';
    const date = new Date(dateStr);
    return date.toLocaleString();
}

function formatDuration(seconds) {
    if (!seconds) return 'N/A';
    if (seconds < 60) return `${seconds}s`;
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`;
    return `${Math.floor(seconds / 86400)}d`;
}

function getPriorityLabel(priority) {
    switch (priority) {
        case 2: return 'High';
        case 1: return 'Normal';
        case 0: return 'Low';
        default: return 'Normal';
    }
}

function showNotification(message, type = 'success') {
    const notification = document.createElement('div');
    notification.className = `notification ${type}`;
    notification.textContent = message;
    document.body.appendChild(notification);

    setTimeout(() => {
        notification.remove();
    }, 3000);
}

function refreshJobs() {
    loadJobs();
    loadMetrics();
    loadQueues();
    loadSchedules();
    showNotification('Refreshed', 'success');
}

// Cleanup on page unload
window.addEventListener('beforeunload', () => {
    if (refreshInterval) {
        clearInterval(refreshInterval);
    }
});
