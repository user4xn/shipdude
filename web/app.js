// Cache for github repositories list
let githubRepos = [];
let detectedStackInfo = null;

// unwrap pulls the payload out of the uniform API envelope
// ({ message, status, data }). Error envelopes carry no data, so the envelope
// itself is returned in that case, letting callers read `message`/`error`.
// Non-enveloped (legacy) responses are returned unchanged.
function unwrap(payload) {
    if (payload && typeof payload === 'object' && 'status' in payload && 'data' in payload) {
        return payload.data !== null && payload.data !== undefined ? payload.data : payload;
    }
    return payload;
}

// errorMessage prefers the enveloped message, then the legacy error field.
function errorMessage(payload, fallback) {
    if (payload && typeof payload === 'object') {
        return payload.message || payload.error || fallback;
    }
    return fallback;
}


// Initialize on page load
document.addEventListener('DOMContentLoaded', () => {
    checkLocalhostWebhookWarning();
    checkSettings();
    loadLastVPSConnection();
    setupPrivateKeyMasking('private_key');
    setupPrivateKeyMasking('deploy_private_key');
    loadProjects();
    
    // Initialize guide state
    if (localStorage.getItem('shipdude_guide_expanded') === 'true') {
        const content = document.getElementById('guide-content');
        const arrow = document.getElementById('guide-arrow');
        if (content && arrow) {
            content.classList.remove('hidden');
            arrow.textContent = '▲';
        }
    }
});

function checkLocalhostWebhookWarning() {
    const hostname = window.location.hostname;
    const warningBanner = document.getElementById('localhost-webhook-warning');
    if (!warningBanner) return;
    if (hostname === 'localhost' || hostname === '127.0.0.1' || hostname.startsWith('192.168.') || hostname.startsWith('10.')) {
        warningBanner.classList.remove('hidden');
    } else {
        warningBanner.classList.add('hidden');
    }
}

function toggleAuthFields() {
    const authMethod = document.getElementById('auth_method').value;
    const passwordGroup = document.getElementById('password-group');
    const keyGroup = document.getElementById('key-group');

    if (authMethod === 'password') {
        passwordGroup.classList.remove('hidden');
        keyGroup.classList.add('hidden');
    } else {
        passwordGroup.classList.add('hidden');
        keyGroup.classList.remove('hidden');
    }
}

function toggleDeployAuthFields() {
    const authMethod = document.getElementById('deploy_auth_method').value;
    const passwordGroup = document.getElementById('deploy-password-group');
    const keyGroup = document.getElementById('deploy-key-group');

    if (authMethod === 'password') {
        passwordGroup.classList.remove('hidden');
        keyGroup.classList.add('hidden');
    } else {
        passwordGroup.classList.add('hidden');
        keyGroup.classList.remove('hidden');
    }
}

// Check Settings status from Go Backend
async function checkSettings() {
    updateGitHubIndicator('checking');
    try {
        const response = await fetch('/api/v1/settings/github');
        const settings = unwrap(await response.json());

        const banner = document.getElementById('github-unconfigured-banner');
        const oauthPrompt = document.getElementById('github-oauth-prompt');

        if (!settings.configured) {
            banner.classList.remove('hidden');
            oauthPrompt.classList.add('hidden');
            updateGitHubIndicator('unconfigured');
            githubRepos = [];
            return;
        }

        banner.classList.add('hidden');
        
        // Settings configured, now check if logged in
        await checkGitHubLogin();
    } catch (err) {
        console.error('Failed to verify backend settings status:', err);
        updateGitHubIndicator('error');
    }
}

async function checkGitHubLogin() {
    const oauthPrompt = document.getElementById('github-oauth-prompt');
    try {
        const userResponse = await fetch('/api/v1/github/user');
        if (userResponse.status === 401) {
            oauthPrompt.classList.remove('hidden');
            updateGitHubIndicator('unauthorized');
            githubRepos = [];
            return;
        }

        if (!userResponse.ok) {
            throw new Error('Failed to get profile');
        }

        const user = unwrap(await userResponse.json());
        oauthPrompt.classList.add('hidden');
        updateGitHubIndicator('connected', user.login);

        // Fetch repo list for autocomplete cache
        const reposResponse = await fetch('/api/v1/github/repos');
        if (reposResponse.ok) {
            githubRepos = unwrap(await reposResponse.json());
        }
    } catch (err) {
        console.error('Failed to list repos:', err);
        oauthPrompt.classList.remove('hidden');
        updateGitHubIndicator('unauthorized');
    }
}

function updateGitHubIndicator(state, username = '') {
    const dot = document.getElementById('github-status-dot');
    const text = document.getElementById('github-status-text');
    const discBtn = document.getElementById('github-disconnect-btn');
    if (!dot || !text) return;

    dot.className = 'w-2 h-2 rounded-full transition-all duration-300';
    if (discBtn) discBtn.classList.add('hidden');
    
    if (state === 'checking') {
        dot.classList.add('bg-slate-500', 'animate-pulse');
        text.textContent = 'Checking GitHub Connection...';
    } else if (state === 'unconfigured') {
        dot.classList.add('bg-rose-500', 'animate-pulse');
        text.textContent = 'GitHub Disconnected (Credentials Missing)';
    } else if (state === 'unauthorized') {
        dot.classList.add('bg-amber-500', 'animate-pulse');
        text.textContent = 'GitHub Configured (Authorization Required)';
    } else if (state === 'connected') {
        dot.classList.add('bg-emerald-500', 'shadow-[0_0_8px_rgba(16,185,129,0.7)]');
        text.innerHTML = `Connected to GitHub as <span class="text-emerald-400 font-semibold">@${username}</span>`;
        if (discBtn) discBtn.classList.remove('hidden');
    } else {
        dot.classList.add('bg-slate-500');
        text.textContent = 'GitHub connection error';
    }
}

async function loadLastVPSConnection() {
    try {
        const response = await fetch('/api/v1/vps/last-connection');
        if (!response.ok) return;
        const data = unwrap(await response.json());
        
        if (data.ip_address) {
            document.getElementById('ip_address').value = data.ip_address;
            document.getElementById('ssh_port').value = data.ssh_port;
            document.getElementById('ssh_user').value = data.ssh_user;
            document.getElementById('auth_method').value = data.auth_method || 'password';
            document.getElementById('password').value = data.password || '';
            setPrivateKeyVal('private_key', data.private_key || '');
            toggleAuthFields();

            // Also auto-populate deploy form
            document.getElementById('deploy_ip').value = data.ip_address;
            document.getElementById('deploy_port').value = data.ssh_port;
            document.getElementById('deploy_user').value = data.ssh_user;
            document.getElementById('deploy_auth_method').value = data.auth_method || 'password';
            document.getElementById('deploy_password').value = data.password || '';
            setPrivateKeyVal('deploy_private_key', data.private_key || '');
            toggleDeployAuthFields();
        }
    } catch (err) {
        console.error('Failed to load last connection:', err);
    }
}

// Save GitHub integration details
async function saveSettings() {
    const clientId = document.getElementById('settings_client_id').value.trim();
    const clientSecret = document.getElementById('settings_client_secret').value.trim();

    try {
        const response = await fetch('/api/v1/settings/github', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ client_id: clientId, client_secret: clientSecret })
        });

        const result = await response.json();
        if (!response.ok) {
            throw new Error(errorMessage(result, 'Failed to save settings'));
        }

        showToast('GitHub configuration updated successfully!', 'success');
        toggleSettingsModal(false);
        checkSettings();
    } catch (err) {
        showToast('Settings error: ' + err.message, 'error');
    }
}

function toggleSettingsModal(show) {
    const modal = document.getElementById('settings-modal');
    if (show) {
        modal.classList.remove('hidden');
    } else {
        modal.classList.add('hidden');
    }
}

// Search repositories autocomplete
function searchRepos(query) {
    const container = document.getElementById('repo-autocomplete');
    if (!query) {
        container.classList.add('hidden');
        return;
    }

    const matches = githubRepos.filter(r => r.full_name.toLowerCase().includes(query.toLowerCase()));
    if (matches.length === 0) {
        container.classList.add('hidden');
        return;
    }

    container.innerHTML = '';
    matches.forEach(repo => {
        const div = document.createElement('div');
        div.className = 'px-4 py-3 hover:bg-white/5 cursor-pointer text-slate-200 text-xs border-b border-white/5 last:border-b-0 transition-colors';
        div.textContent = repo.full_name;
        div.onclick = () => selectRepo(repo);
        container.appendChild(div);
    });

    container.classList.remove('hidden');
}

function selectRepo(repo) {
    document.getElementById('github_repo').value = repo.full_name;
    document.getElementById('repo-autocomplete').classList.add('hidden');
    
    // Auto-populate deployment project fields
    const repoName = repo.full_name.split('/')[1] || '';
    document.getElementById('deploy_project_name').value = repoName;
    document.getElementById('deploy_path').value = `/var/www/${repoName}`;

    // Load branches for this repo
    fetchBranches(repo.full_name, repo.default_branch || 'main');

    // Auto-detect stack on repository selection
    detectStack();
}

async function probeVPS() {
    const ip = document.getElementById('ip_address').value.trim();
    const port = parseInt(document.getElementById('ssh_port').value, 10);
    const user = document.getElementById('ssh_user').value.trim();
    const authMethod = document.getElementById('auth_method').value;
    const password = document.getElementById('password').value;
    const privateKey = getPrivateKeyVal('private_key');

    if (!ip || !user) {
        showToast('Please enter VPS Host IP and SSH Username', 'warning');
        return;
    }

    const btn = document.querySelector('#vps-form button');
    const spinner = document.getElementById('vps-spinner');
    const diagBox = document.getElementById('vps-diagnostics');

    btn.disabled = true;
    spinner.classList.remove('hidden');
    diagBox.classList.remove('hidden');

    updateField('vps-status-connected', 'Checking...', 'px-2.5 py-0.5 rounded-full text-xs font-semibold uppercase bg-amber-500/10 text-amber-400 border border-amber-500/25');
    updateField('vps-status-os', 'Checking...', 'font-semibold text-slate-400');
    updateField('vps-status-docker', 'Checking...', 'font-semibold text-slate-400');
    updateField('vps-status-compose', 'Checking...', 'font-semibold text-slate-400');
    updateField('vps-status-sudo', 'Checking...', 'font-semibold text-slate-400');
    updateField('vps-status-group', 'Checking...', 'font-semibold text-slate-400');
    updateField('vps-status-bootstrapped', 'Checking...', 'font-semibold text-slate-400');
    const bootBadge = document.getElementById('bootstrap-status-badge');
    if (bootBadge) bootBadge.classList.add('hidden');

    try {
        const payload = {
            ip_address: ip,
            ssh_port: port,
            ssh_user: user,
            auth_method: authMethod
        };

        if (authMethod === 'password') {
            payload.password = password;
        } else {
            payload.private_key = privateKey;
        }

        const response = await fetch('/api/v1/vps/probe', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });

        const result = await response.json();
        if (!response.ok) {
            throw new Error(errorMessage(result, 'Server returned an error'));
        }

        if (result.connected) {
            updateField('vps-status-connected', 'Connected', 'px-2.5 py-0.5 rounded-full text-xs font-semibold uppercase bg-emerald-500/10 text-emerald-400 border border-emerald-500/25');
            updateField('vps-status-os', result.os || 'Linux', 'font-semibold text-slate-200');
            
            const docVer = result.docker_version;
            updateField('vps-status-docker', docVer, docVer.startsWith('Docker') ? 'font-semibold text-emerald-400' : 'font-semibold text-rose-400');

            const compVer = result.compose_version;
            updateField('vps-status-compose', compVer, compVer.startsWith('Docker') ? 'font-semibold text-emerald-400' : 'font-semibold text-rose-400');

            updateField('vps-status-sudo', result.has_sudo ? 'Granted' : 'Denied', result.has_sudo ? 'font-semibold text-emerald-400' : 'font-semibold text-rose-400');
            updateField('vps-status-group', result.in_docker_group ? 'Yes' : 'No', result.in_docker_group ? 'font-semibold text-emerald-400' : 'font-semibold text-rose-400');
            updateField('vps-status-bootstrapped', result.bootstrapped ? 'Ready (Existing)' : 'Not Found', result.bootstrapped ? 'font-semibold text-emerald-400' : 'font-semibold text-amber-400');
            if (bootBadge) {
                if (result.bootstrapped) {
                    bootBadge.classList.remove('hidden');
                } else {
                    bootBadge.classList.add('hidden');
                }
            }
            showToast('VPS Connection & Diagnostics successfully completed!', 'success');
        } else {
            updateField('vps-status-connected', 'Failed', 'px-2.5 py-0.5 rounded-full text-xs font-semibold uppercase bg-rose-500/10 text-rose-400 border border-rose-500/25');
            updateField('vps-status-os', '-', 'font-semibold text-slate-400');
            updateField('vps-status-docker', '-', 'font-semibold text-slate-400');
            updateField('vps-status-compose', '-', 'font-semibold text-slate-400');
            updateField('vps-status-sudo', '-', 'font-semibold text-slate-400');
            updateField('vps-status-group', '-', 'font-semibold text-slate-400');
            updateField('vps-status-bootstrapped', '-', 'font-semibold text-slate-400');
            if (bootBadge) bootBadge.classList.add('hidden');
            showToast('Connection failed: ' + (result.error || 'Unreachable'), 'error');
        }
    } catch (err) {
        showToast('Error: ' + err.message, 'error');
        updateField('vps-status-connected', 'Error', 'px-2.5 py-0.5 rounded-full text-xs font-semibold uppercase bg-rose-500/10 text-rose-400 border border-rose-500/25');
    } finally {
        btn.disabled = false;
        spinner.classList.add('hidden');
    }
}

async function detectStack() {
    const repo = document.getElementById('github_repo').value.trim();
    const token = document.getElementById('github_token').value.trim();

    if (!repo) {
        showToast('Please select a GitHub repository', 'warning');
        return;
    }

    const btn = document.querySelector('#github-form button');
    const spinner = document.getElementById('git-spinner');
    const diagBox = document.getElementById('git-diagnostics');

    btn.disabled = true;
    spinner.classList.remove('hidden');
    diagBox.classList.remove('hidden');

    try {
        const response = await fetch('/api/v1/projects/detect-stack', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ repo, token })
        });

        const result = await response.json();
        if (!response.ok) {
            throw new Error(errorMessage(result, 'Failed to detect stack'));
        }

        detectedStackInfo = result;

        const logo = document.getElementById('tech-logo-char');
        logo.className = 'w-12 h-12 rounded-xl bg-white/[0.03] border border-white/5 flex items-center justify-center text-lg font-bold transition-colors'; 

        let logoChar = '?';
        if (result.language === 'go') {
            logoChar = 'Go';
            logo.classList.add('bg-blue-500/10', 'border-blue-500/20', 'text-blue-400');
        } else if (result.language === 'node') {
            logoChar = 'JS';
            logo.classList.add('bg-emerald-500/10', 'border-emerald-500/20', 'text-emerald-400');
        } else if (result.language === 'python') {
            logoChar = 'Py';
            logo.classList.add('bg-yellow-500/10', 'border-yellow-500/20', 'text-yellow-400');
        }

        logo.textContent = logoChar;
        document.getElementById('tech-language').textContent = result.language;
        document.getElementById('tech-version').textContent = 'Base Tag: ' + result.version;
        document.getElementById('tech-port').textContent = result.default_port;
        document.getElementById('tech-files').textContent = result.detected_files.join(', ') || 'None';

        const elEligibility = document.getElementById('tech-eligibility');
        const warnBox = document.getElementById('eligibility-warning');
        const deployBtn = document.getElementById('deploy-submit-btn');

        if (result.has_dockerfile) {
            elEligibility.textContent = 'Eligible';
            elEligibility.className = 'val font-semibold px-2 px-2.5 py-1 rounded text-xs bg-emerald-500/10 border border-emerald-500/20 text-emerald-400';
            warnBox.classList.add('hidden');
            if (deployBtn) {
                deployBtn.disabled = false;
                deployBtn.classList.remove('opacity-50', 'cursor-not-allowed', 'pointer-events-none');
            }
        } else {
            elEligibility.textContent = 'Not Eligible';
            elEligibility.className = 'val font-semibold px-2 px-2.5 py-1 rounded text-xs bg-red-500/10 border border-red-500/20 text-red-400';
            warnBox.classList.remove('hidden');
            if (deployBtn) {
                deployBtn.disabled = true;
                deployBtn.classList.add('opacity-50', 'cursor-not-allowed', 'pointer-events-none');
            }
        }

        // Auto-fill deploy page properties
        document.getElementById('deploy_stack_info').value = `${result.language} (version ${result.version})`;
        document.getElementById('deploy_port_val').value = result.default_port;

        const repoParts = repo.split('/');
        const repoName = repoParts[repoParts.length - 1];
        if (repoName) {
            document.getElementById('deploy_project_name').value = repoName;
            document.getElementById('deploy_path').value = `/var/www/${repoName}`;
        }

        // Fetch branches for manually entered repos
        fetchBranches(repo);

        showToast('Stack Discovery successfully completed!', 'success');
    } catch (err) {
        showToast('Detection error: ' + err.message, 'error');
        document.getElementById('tech-language').textContent = 'Unknown';
        document.getElementById('tech-version').textContent = 'Version: -';
        document.getElementById('tech-port').textContent = '-';
        document.getElementById('tech-files').textContent = '-';
        
        const elEligibility = document.getElementById('tech-eligibility');
        if (elEligibility) {
            elEligibility.textContent = '-';
            elEligibility.className = 'val font-semibold px-2 px-2.5 py-1 rounded text-xs';
        }
        const warnBox = document.getElementById('eligibility-warning');
        if (warnBox) warnBox.classList.add('hidden');
        const deployBtn = document.getElementById('deploy-submit-btn');
        if (deployBtn) {
            deployBtn.disabled = false;
            deployBtn.classList.remove('opacity-50', 'cursor-not-allowed', 'pointer-events-none');
        }

        const logo = document.getElementById('tech-logo-char');
        logo.className = 'w-12 h-12 rounded-xl bg-white/[0.03] border border-white/5 flex items-center justify-center text-lg font-bold transition-colors';
        logo.textContent = '?';
    } finally {
        btn.disabled = false;
        spinner.classList.add('hidden');
    }
}

// Copy connection credential fields from probe to deploy form
function copyVPSDetails() {
    document.getElementById('deploy_ip').value = document.getElementById('ip_address').value;
    document.getElementById('deploy_port').value = document.getElementById('ssh_port').value;
    document.getElementById('deploy_user').value = document.getElementById('ssh_user').value;
    
    const authMethod = document.getElementById('auth_method').value;
    document.getElementById('deploy_auth_method').value = authMethod;
    toggleDeployAuthFields();

    if (authMethod === 'password') {
        document.getElementById('deploy_password').value = document.getElementById('password').value;
    } else {
        setPrivateKeyVal('deploy_private_key', getPrivateKeyVal('private_key'));
    }

    showToast('Credentials copied successfully!', 'success');
}

// Trigger Deploy POST to setup backend CI/CD integration
async function triggerDeploy() {
    const payload = {
        ip_address: document.getElementById('deploy_ip').value.trim(),
        ssh_port: parseInt(document.getElementById('deploy_port').value, 10),
        ssh_user: document.getElementById('deploy_user').value.trim(),
        auth_method: document.getElementById('deploy_auth_method').value,
        github_repo: document.getElementById('github_repo').value.trim(),
        github_branch: document.getElementById('deploy_branch').value.trim(),
        project_name: document.getElementById('deploy_project_name').value.trim(),
        deployment_path: document.getElementById('deploy_path').value.trim(),
        app_port: parseInt(document.getElementById('deploy_port_val').value, 10),
        github_token: document.getElementById('github_token').value.trim(),
    };

    if (payload.auth_method === 'password') {
        payload.password = document.getElementById('deploy_password').value;
    } else {
        payload.private_key = getPrivateKeyVal('deploy_private_key');
    }

    if (detectedStackInfo) {
        payload.stack_type = detectedStackInfo.language;
        payload.stack_version = detectedStackInfo.version;
    } else {
        payload.stack_type = 'unknown';
        payload.stack_version = 'latest';
    }

    const btn = document.querySelector('#deploy-form button');
    const spinner = document.getElementById('deploy-spinner');

    btn.disabled = true;
    spinner.classList.remove('hidden');

    try {
        const response = await fetch('/api/v1/projects/deploy', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });

        const result = await response.json();
        if (!response.ok) {
            throw new Error(errorMessage(result, 'Failed to deploy project'));
        }

        showToast('Deployment config pipeline started!', 'success');
        openTerminal('deploy', payload.project_name);
    } catch (err) {
        showToast('Deployment configuration failed: ' + err.message, 'error');
    } finally {
        btn.disabled = false;
        spinner.classList.add('hidden');
    }
}

function updateField(elementId, value, extraClass = '') {
    const el = document.getElementById(elementId);
    if (!el) return;
    const valSpan = el.querySelector('.val');
    if (valSpan) {
        valSpan.textContent = value;
        if (extraClass) {
            valSpan.className = 'val ' + extraClass;
        }
    }
}

function showToast(message, type = 'info') {
    const container = document.getElementById('toast-container');
    const toast = document.createElement('div');
    toast.className = 'bg-slate-900 border border-white/5 border-l-4 rounded-xl px-5 py-3.5 text-slate-200 text-sm shadow-2xl backdrop-blur-md min-w-[300px] flex items-center justify-between transition-all duration-300 transform translate-y-2 opacity-0';
    
    if (type === 'success') {
        toast.classList.add('border-l-emerald-500');
    } else if (type === 'error') {
        toast.classList.add('border-l-rose-500');
    } else if (type === 'warning') {
        toast.classList.add('border-l-amber-500');
    } else {
        toast.classList.add('border-l-blue-500');
    }

    toast.textContent = message;
    container.appendChild(toast);

    setTimeout(() => {
        toast.classList.remove('translate-y-2', 'opacity-0');
        toast.classList.add('translate-y-0', 'opacity-100');
    }, 10);

    setTimeout(() => {
        toast.classList.remove('translate-y-0', 'opacity-100');
        toast.classList.add('translate-y-[-10px]', 'opacity-0');
        toast.addEventListener('transitionend', () => {
            toast.remove();
        });
    }, 4000);
}

async function saveVPSConnectionManual() {
    const payload = {
        ip_address: document.getElementById('ip_address').value.trim(),
        ssh_port: parseInt(document.getElementById('ssh_port').value, 10) || 22,
        ssh_user: document.getElementById('ssh_user').value.trim(),
        auth_method: document.getElementById('auth_method').value,
        password: document.getElementById('password').value,
        private_key: getPrivateKeyVal('private_key'),
    };

    if (!payload.ip_address || !payload.ssh_user) {
        showToast('IP Address and SSH User are required to save credentials', 'warning');
        return;
    }

    try {
        const response = await fetch('/api/v1/vps/last-connection', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });

        const result = await response.json();
        if (!response.ok) {
            throw new Error(errorMessage(result, 'Failed to save connection credentials'));
        }

        showToast('VPS credentials saved successfully!', 'success');

        // Populate deploy form too
        document.getElementById('deploy_ip').value = payload.ip_address;
        document.getElementById('deploy_port').value = payload.ssh_port;
        document.getElementById('deploy_user').value = payload.ssh_user;
        document.getElementById('deploy_auth_method').value = payload.auth_method;
        document.getElementById('deploy_password').value = payload.password;
        setPrivateKeyVal('deploy_private_key', payload.private_key);
        toggleDeployAuthFields();
    } catch (err) {
        showToast(err.message, 'error');
    }
}

async function disconnectGitHub() {
    const ok = await showConfirm(
        'Disconnect GitHub',
        'Are you sure you want to disconnect your GitHub integration? This will clear your access token.',
        { type: 'danger', okText: 'Disconnect', icon: '🔌' }
    );
    if (!ok) {
        return;
    }

    try {
        const response = await fetch('/api/v1/github/disconnect', {
            method: 'POST'
        });

        const result = await response.json();
        if (!response.ok) {
            throw new Error(errorMessage(result, 'Failed to disconnect from GitHub'));
        }

        showToast('Successfully disconnected from GitHub!', 'success');
        
        // Hide disconnect button and trigger settings refresh
        const discBtn = document.getElementById('github-disconnect-btn');
        if (discBtn) discBtn.classList.add('hidden');

        githubRepos = [];
        const container = document.getElementById('repo-autocomplete');
        if (container) {
            container.innerHTML = '';
            container.classList.add('hidden');
        }

        await checkSettings();
    } catch (err) {
        showToast(err.message, 'error');
    }
}



async function fetchBranches(repo, defaultBranch = 'main') {
    if (!repo) return;
    const token = document.getElementById('github_token').value.trim();
    try {
        let url = `/api/v1/github/branches?repo=${encodeURIComponent(repo)}`;
        if (token) {
            url += `&token=${encodeURIComponent(token)}`;
        }
        const response = await fetch(url);
        if (!response.ok) {
            throw new Error('Failed to fetch branches');
        }
        const branches = unwrap(await response.json());
        const select = document.getElementById('deploy_branch');
        if (!select) return;
        
        select.innerHTML = '';
        branches.forEach(b => {
            const opt = document.createElement('option');
            opt.value = b.name;
            opt.textContent = b.name;
            select.appendChild(opt);
        });

        // Try to select defaultBranch if it exists in the list
        const hasDefault = branches.some(b => b.name === defaultBranch);
        if (hasDefault) {
            select.value = defaultBranch;
        } else if (branches.length > 0) {
            select.value = branches[0].name;
        }
    } catch (err) {
        console.error('Failed to load branches:', err);
    }
}

function setupPrivateKeyMasking(elementId) {
    const el = document.getElementById(elementId);
    if (!el) return;

    el._actualValue = el.value || '';

    el.addEventListener('focus', () => {
        el.value = el._actualValue || '';
    });

    el.addEventListener('blur', () => {
        el._actualValue = el.value || '';
        if (el.value.trim() !== '') {
            el.value = '★'.repeat(64) + '\n[SSH Private Key Masked for Security]';
        }
    });

    el.addEventListener('input', () => {
        el._actualValue = el.value || '';
    });
}

function getPrivateKeyVal(elementId) {
    const el = document.getElementById(elementId);
    if (!el) return '';
    return el._actualValue !== undefined ? el._actualValue : el.value;
}

function setPrivateKeyVal(elementId, val) {
    const el = document.getElementById(elementId);
    if (!el) return;
    el._actualValue = val || '';
    if (document.activeElement === el) {
        el.value = el._actualValue;
    } else {
        if (el._actualValue.trim() !== '') {
            el.value = '★'.repeat(64) + '\n[SSH Private Key Masked for Security]';
        } else {
            el.value = '';
        }
    }
}

let ws = null;

async function loadProjects() {
    try {
        const response = await fetch('/api/v1/projects');
        if (!response.ok) {
            throw new Error('Failed to load projects');
        }
        const projects = unwrap(await response.json());
        const container = document.getElementById('projects-list-container');
        if (!container) return;

        if (!projects || projects.length === 0) {
            container.innerHTML = `<p class="col-span-1 md:col-span-2 text-slate-400 text-sm italic">No active projects registered. Use the configuration form above to launch your first deployment.</p>`;
            return;
        }

        container.innerHTML = '';
        projects.forEach(p => {
            const cleanName = p.project_name.toLowerCase().replace(/[^a-z0-9]/g, '');
            const domain = p.domain || `${cleanName}.${p.server_ip || 'vps'}.sslip.io`;
            const image = p.active_image || 'No deployment active';
            const port = p.active_port || 'None';

            const card = document.createElement('div');
            card.className = 'bg-white/[0.02] border border-white/5 rounded-2xl p-6 hover:border-white/10 transition-all duration-300 flex flex-col justify-between';
            card.innerHTML = `
                <div>
                    <div class="flex justify-between items-start mb-4">
                        <div>
                            <h3 class="text-md font-bold text-slate-100">${p.project_name}</h3>
                            <p class="text-xs text-slate-400 mt-0.5">${p.github_repo} (${p.github_branch})</p>
                        </div>
                        <div class="flex items-center gap-2">
                            <span class="px-2.5 py-1 rounded-full text-[10px] font-mono font-bold uppercase ${p.active_port ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20' : 'bg-slate-500/10 text-slate-400 border border-white/5'}">
                                ${p.active_port ? 'Active' : 'Configured'}
                            </span>
                            <button onclick="deleteProject(${p.id})" title="Delete Project Config" class="p-1 rounded-lg text-slate-400 hover:text-rose-400 hover:bg-rose-500/10 active:scale-95 transition-all">
                                🗑️
                            </button>
                        </div>
                    </div>
                    
                    <ul class="space-y-2.5 text-xs text-slate-300">
                        <li class="flex justify-between py-1 border-b border-white/[0.03]">
                            <span class="text-slate-400">Target Ingress Inbound</span>
                            <a href="http://${domain}" target="_blank" class="text-blue-400 hover:underline hover:text-blue-300 transition-colors font-mono">${domain} ↗</a>
                        </li>
                        <li class="flex justify-between py-1 border-b border-white/[0.03]">
                            <span class="text-slate-400">Upstream Host Port</span>
                            <span class="font-mono font-semibold">${port}</span>
                        </li>
                        <li class="flex justify-between py-1 border-b border-white/[0.03]">
                            <span class="text-slate-400">Active Container Image</span>
                            <span class="font-mono text-slate-400 select-all truncate max-w-[200px]" title="${image}">${image}</span>
                        </li>
                    </ul>
                </div>

                <div class="mt-6 pt-4 border-t border-white/5 space-y-4">
                    <div class="flex gap-2">
                        <input type="text" id="rollback-image-${p.id}" placeholder="ghcr.io/username/repo:sha-12345" 
                            class="flex-1 bg-slate-900 border border-white/5 rounded-xl text-xs px-3 py-2 text-slate-100 focus:outline-none focus:border-rose-500 transition-colors">
                        <button onclick="rollbackProject(${p.id})" class="bg-rose-500/20 hover:bg-rose-500 text-rose-300 hover:text-white text-xs font-semibold px-4 py-2 rounded-xl transition-all border border-rose-500/20 active:scale-95">
                            Rollback
                        </button>
                    </div>
                    
                    ${p.active_port ? `
                    <button onclick="openMonitoring(${p.id}, '${p.project_name}')" class="w-full bg-gradient-to-r from-blue-500/10 to-purple-500/10 hover:from-blue-500/20 hover:to-purple-500/20 hover:border-blue-500/30 border border-blue-500/10 text-slate-200 hover:text-white text-xs font-semibold py-2.5 rounded-xl transition-all flex items-center justify-center gap-1.5 shadow-md active:scale-95">
                        📊 Open Monitoring Dashboard
                    </button>
                    ` : ''}
                    <button onclick="openTerminal('project', ${p.id})" class="w-full bg-white/5 hover:bg-white/10 active:scale-95 text-slate-300 hover:text-white text-xs font-semibold py-2.5 rounded-xl border border-white/5 transition-all flex items-center justify-center gap-1.5 shadow-md">
                        📺 Open Execution Logs Console
                    </button>
                </div>
            `;
            container.appendChild(card);
        });
    } catch (err) {
        console.error('Failed to load projects:', err);
    }
}

async function deleteProject(id) {
    const ok = await showConfirm(
        'Delete Project',
        'Are you sure you want to delete this project configuration? This will only remove the configuration from ShipDude, it will not stop or delete any running containers on your VPS.',
        { type: 'danger', okText: 'Delete', icon: '🗑️' }
    );
    if (!ok) {
        return;
    }
    try {
        const response = await fetch(`/api/v1/projects/${id}`, {
            method: 'DELETE'
        });
        if (!response.ok) {
            const payload = await response.json();
            throw new Error(errorMessage(payload, 'Failed to delete project'));
        }
        showToast('Project configuration deleted successfully', 'success');
        loadProjects();
    } catch (err) {
        showToast(err.message, 'error');
    }
}

async function provisionVPSPrompt() {
    const ip = document.getElementById('ip_address').value.trim();
    const port = parseInt(document.getElementById('ssh_port').value, 10);
    const user = document.getElementById('ssh_user').value.trim();
    const authMethod = document.getElementById('auth_method').value;
    const password = document.getElementById('password').value;
    const privateKey = getPrivateKeyVal('private_key');

    if (!ip || !user) {
        showToast('Please enter VPS Host IP and SSH Username', 'warning');
        return;
    }

    const ok = await showConfirm(
        'Provision VPS Node',
        'Are you sure you want to provision and bootstrap this VPS? This will install Docker and setup deployment directories.',
        { type: 'warning', okText: 'Bootstrap', icon: '⚙️' }
    );
    if (!ok) {
        return;
    }

    const spinner = document.getElementById('provision-spinner');
    spinner.classList.remove('hidden');

    try {
        const payload = {
            ip_address: ip,
            ssh_port: port,
            ssh_user: user,
            auth_method: authMethod
        };

        if (authMethod === 'password') {
            payload.password = password;
        } else {
            payload.private_key = privateKey;
        }

        const response = await fetch('/api/v1/vps/provision', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });

        const result = await response.json();
        if (!response.ok) {
            throw new Error(errorMessage(result, 'Server provisioning initiation failed'));
        }

        showToast('VPS Provisioning started! Streaming logs...', 'success');
        openTerminal('provision', ip);
    } catch (err) {
        showToast(err.message, 'error');
    } finally {
        spinner.classList.add('hidden');
    }
}

async function rollbackProject(id) {
    const imgEl = document.getElementById(`rollback-image-${id}`);
    const imgVal = imgEl ? imgEl.value.trim() : '';

    if (!imgVal) {
        showToast('Please enter a target image tag/SHA to rollback', 'warning');
        return;
    }

    const ok = await showConfirm(
        'Deploy / Rollback Image',
        `Are you sure you want to deploy image "${imgVal}"?`,
        { type: 'warning', okText: 'Deploy', icon: '🚀' }
    );
    if (!ok) {
        return;
    }

    try {
        const response = await fetch(`/api/v1/projects/${id}/rollback`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ target_image: imgVal })
        });

        const result = await response.json();
        if (!response.ok) {
            throw new Error(errorMessage(result, 'Rollback execution failed'));
        }

        showToast('Rollback/Redeployment started! Streaming logs...', 'success');
        openTerminal('project', id);
        if (imgEl) imgEl.value = '';
    } catch (err) {
        showToast(err.message, 'error');
    }
}

function openTerminal(type, idOrIp) {
    const modal = document.getElementById('terminal-modal');
    const content = document.getElementById('terminal-content');
    const title = document.getElementById('terminal-title');

    if (!modal || !content) return;

    modal.classList.remove('hidden');
    content.innerHTML = '<div class="text-slate-500">// Connecting to server log stream...</div>';

    if (ws) {
        ws.close();
    }

    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    let wsUrl = '';

    if (type === 'provision') {
        title.textContent = `VPS Provisioning: ${idOrIp}`;
        wsUrl = `${proto}//${window.location.host}/api/v1/vps/provision/logs?ip=${encodeURIComponent(idOrIp)}`;
    } else if (type === 'deploy') {
        title.textContent = `Project Deploy Config: ${idOrIp}`;
        wsUrl = `${proto}//${window.location.host}/api/v1/projects/deploy/logs?project_name=${encodeURIComponent(idOrIp)}`;
    } else {
        title.textContent = `Deployment Console (ID: ${idOrIp})`;
        wsUrl = `${proto}//${window.location.host}/api/v1/projects/${idOrIp}/logs`;
    }

    ws = new WebSocket(wsUrl);

    ws.onopen = () => {
        content.innerHTML += '<div class="text-emerald-500 font-semibold">[Connected] Listening to remote pipeline stream...</div>';
    };

    ws.onmessage = (event) => {
        appendTerminalLine(event.data);
    };

    ws.onclose = (event) => {
        content.innerHTML += `<div class="text-amber-500 font-semibold">[Disconnected] Stream closed. (Code: ${event.code})</div>`;
        ws = null;
        loadProjects(); // Reload projects list to reflect new deployment statuses
        if (type === 'provision') {
            probeVPS(); // Auto-refresh VPS diagnostics to show updated status!
        }
    };

    ws.onerror = (err) => {
        content.innerHTML += '<div class="text-rose-500 font-semibold">[Error] Stream connection failed.</div>';
        console.error(err);
    };
}

function closeTerminal() {
    const modal = document.getElementById('terminal-modal');
    if (modal) {
        modal.classList.add('hidden');
    }
    if (ws) {
        ws.close();
        ws = null;
    }
}

function appendTerminalLine(text) {
    const content = document.getElementById('terminal-content');
    if (!content) return;

    const line = document.createElement('div');
    line.textContent = text;
    content.appendChild(line);

    // Keep it scrolled to bottom
    content.scrollTop = content.scrollHeight;
}

let monitorTelemetryWs = null;
let monitorLogsWs = null;
let activeMonitorProjectId = null;
let containerIsOnline = false;

function openMonitoring(projectId, projectName) {
    activeMonitorProjectId = projectId;
    const modal = document.getElementById('monitoring-modal');
    if (!modal) return;

    modal.classList.remove('hidden');
    document.getElementById('monitor-title').textContent = `Project Monitoring: ${projectName}`;
    
    // Reset fields
    document.getElementById('monitor-cpu-perc').textContent = '0.0%';
    document.getElementById('monitor-cpu-bar').style.width = '0%';
    document.getElementById('monitor-mem-perc').textContent = '0MB / 0MB (0.0%)';
    document.getElementById('monitor-mem-bar').style.width = '0%';
    
    const logsContainer = document.getElementById('monitor-logs');
    logsContainer.innerHTML = '<div class="text-slate-500">// Connecting to container log stream...</div>';

    // Status fields
    const dot = document.getElementById('monitor-status-dot');
    const text = document.getElementById('monitor-status-text');
    dot.className = 'w-2.5 h-2.5 rounded-full bg-slate-500 animate-pulse';
    text.textContent = 'Status: Connecting...';

    // Disconnect old sockets
    if (monitorTelemetryWs) monitorTelemetryWs.close();
    if (monitorLogsWs) monitorLogsWs.close();

    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    
    // 1. Telemetry connection
    const telemetryUrl = `${proto}//${window.location.host}/api/v1/projects/${projectId}/telemetry`;
    monitorTelemetryWs = new WebSocket(telemetryUrl);
    
    monitorTelemetryWs.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);
            if (data.online) {
                containerIsOnline = true;
                dot.className = 'w-2.5 h-2.5 rounded-full bg-emerald-400 animate-pulse';
                text.textContent = 'Status: Online';
                
                // Stop button styling
                const toggleBtn = document.getElementById('ctrl-toggle-btn');
                toggleBtn.textContent = '🛑 Stop Container';
                toggleBtn.className = 'px-4 py-2 text-xs font-semibold bg-rose-500/10 hover:bg-rose-500 border border-rose-500/20 hover:border-transparent text-rose-400 hover:text-white rounded-xl transition-all duration-300 active:scale-95 flex items-center gap-1.5 shadow-md shadow-rose-500/5';
                
                // Update stats
                document.getElementById('monitor-cpu-perc').textContent = data.cpu_perc + '%';
                document.getElementById('monitor-cpu-bar').style.width = Math.min(100, parseFloat(data.cpu_perc) || 0) + '%';
                
                document.getElementById('monitor-mem-perc').textContent = `${data.mem_usage} (${data.mem_perc}%)`;
                document.getElementById('monitor-mem-bar').style.width = Math.min(100, parseFloat(data.mem_perc) || 0) + '%';
            } else {
                containerIsOnline = false;
                dot.className = 'w-2.5 h-2.5 rounded-full bg-rose-500';
                text.textContent = 'Status: Offline';
                
                // Start button styling
                const toggleBtn = document.getElementById('ctrl-toggle-btn');
                toggleBtn.textContent = '▶️ Start Container';
                toggleBtn.className = 'px-4 py-2 text-xs font-semibold bg-emerald-500/10 hover:bg-emerald-500 border border-emerald-500/20 hover:border-transparent text-emerald-400 hover:text-white rounded-xl transition-all duration-300 active:scale-95 flex items-center gap-1.5 shadow-md shadow-emerald-500/5';
                
                // Clear stats
                document.getElementById('monitor-cpu-perc').textContent = '0.0%';
                document.getElementById('monitor-cpu-bar').style.width = '0%';
                document.getElementById('monitor-mem-perc').textContent = 'Offline';
                document.getElementById('monitor-mem-bar').style.width = '0%';
            }
        } catch (e) {
            console.error('Failed to parse telemetry frame', e);
        }
    };
    
    // 2. Logs connection
    const logsUrl = `${proto}//${window.location.host}/api/v1/projects/${projectId}/container-logs`;
    monitorLogsWs = new WebSocket(logsUrl);
    
    monitorLogsWs.onopen = () => {
        logsContainer.innerHTML = '<div class="text-emerald-500 font-semibold">// Live log stream connected...</div>';
    };
    
    monitorLogsWs.onmessage = (event) => {
        const line = document.createElement('div');
        line.textContent = event.data;
        logsContainer.appendChild(line);
        logsContainer.scrollTop = logsContainer.scrollHeight;
    };
    
    monitorLogsWs.onclose = () => {
        const line = document.createElement('div');
        line.className = 'text-amber-500 font-semibold';
        line.textContent = '// Log stream disconnected.';
        logsContainer.appendChild(line);
    };
}

function closeMonitoringModal() {
    const modal = document.getElementById('monitoring-modal');
    if (modal) {
        modal.classList.add('hidden');
    }
    if (monitorTelemetryWs) {
        monitorTelemetryWs.close();
        monitorTelemetryWs = null;
    }
    if (monitorLogsWs) {
        monitorLogsWs.close();
        monitorLogsWs = null;
    }
    activeMonitorProjectId = null;
}

async function controlContainer(action) {
    if (!activeMonitorProjectId) return;
    
    let targetAction = action;
    if (action === 'toggle') {
        targetAction = containerIsOnline ? 'stop' : 'start';
    }
    
    const btn = targetAction === 'restart' ? document.getElementById('ctrl-restart-btn') : document.getElementById('ctrl-toggle-btn');
    const oldText = btn.textContent;
    btn.disabled = true;
    btn.textContent = 'Processing...';
    
    showToast(`Sending ${targetAction} command to target VPS...`, 'info');
    
    try {
        const response = await fetch(`/api/v1/projects/${activeMonitorProjectId}/control`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ action: targetAction })
        });
        
        const result = await response.json();
        if (!response.ok) {
            throw new Error(errorMessage(result, `Failed to ${targetAction} container`));
        }
        
        showToast(`Container ${targetAction} completed successfully!`, 'success');
    } catch (err) {
        showToast(err.message, 'error');
    } finally {
        btn.disabled = false;
        btn.textContent = oldText;
    }
}

function toggleGuide() {
    const content = document.getElementById('guide-content');
    const arrow = document.getElementById('guide-arrow');
    if (!content || !arrow) return;
    
    if (content.classList.contains('hidden')) {
        content.classList.remove('hidden');
        arrow.textContent = '▲';
        localStorage.setItem('shipdude_guide_expanded', 'true');
    } else {
        content.classList.add('hidden');
        arrow.textContent = '▼';
        localStorage.setItem('shipdude_guide_expanded', 'false');
    }
}

function showConfirm(title, message, options = {}) {
    return new Promise((resolve) => {
        const modal = document.getElementById('confirm-modal');
        const box = document.getElementById('confirm-modal-box');
        const titleEl = document.getElementById('confirm-title');
        const msgEl = document.getElementById('confirm-message');
        const iconEl = document.getElementById('confirm-icon');
        const okBtn = document.getElementById('confirm-ok-btn');
        const cancelBtn = document.getElementById('confirm-cancel-btn');

        if (!modal || !box) {
            resolve(confirm(message));
            return;
        }

        titleEl.textContent = title || 'Confirm Action';
        msgEl.textContent = message || 'Are you sure you want to proceed?';
        iconEl.textContent = options.icon || '⚠️';

        const type = options.type || 'warning';
        if (type === 'danger') {
            iconEl.className = 'w-12 h-12 rounded-2xl bg-rose-500/10 border border-rose-500/20 flex items-center justify-center text-xl shrink-0 text-rose-400';
            okBtn.className = 'flex-1 bg-rose-500 hover:bg-rose-600 active:scale-[0.98] text-white font-bold py-3 rounded-xl text-xs transition-all';
            okBtn.textContent = options.okText || 'Delete';
        } else if (type === 'success') {
            iconEl.className = 'w-12 h-12 rounded-2xl bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-xl shrink-0 text-emerald-400';
            okBtn.className = 'flex-1 bg-emerald-500 hover:bg-emerald-600 active:scale-[0.98] text-slate-950 font-bold py-3 rounded-xl text-xs transition-all';
            okBtn.textContent = options.okText || 'Confirm';
        } else {
            iconEl.className = 'w-12 h-12 rounded-2xl bg-amber-500/10 border border-amber-500/20 flex items-center justify-center text-xl shrink-0 text-amber-400';
            okBtn.className = 'flex-1 bg-amber-500 hover:bg-amber-600 active:scale-[0.98] text-slate-950 font-bold py-3 rounded-xl text-xs transition-all';
            okBtn.textContent = options.okText || 'Confirm';
        }

        cancelBtn.textContent = options.cancelText || 'Cancel';

        modal.classList.remove('hidden');
        setTimeout(() => {
            box.classList.remove('scale-95', 'opacity-0');
            box.classList.add('scale-100', 'opacity-100');
        }, 10);

        function cleanup(result) {
            box.classList.remove('scale-100', 'opacity-100');
            box.classList.add('scale-95', 'opacity-0');
            setTimeout(() => {
                modal.classList.add('hidden');
                resolve(result);
            }, 150);
        }

        okBtn.onclick = () => cleanup(true);
        cancelBtn.onclick = () => cleanup(false);
        modal.onclick = (e) => {
            if (e.target === modal) cleanup(false);
        };
    });
}
