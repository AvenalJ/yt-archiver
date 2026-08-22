// State
let currentTab = 'download-tab';
let downloadMode = 'single';
let inspectedData = null;
let allDownloads = [];
let allChannels = [];
let activeQueueFilter = 'all';
let isQueueRandomized = false;
let randomizedQueueMap = new Map();
let originalQueueOrder = [];
let dismissedFeatureCards = [];
let librarySearchTerm = '';
let eventSource = null;
let downloadProfiles = [];
let preferencesLoaded = false;
let preferencesSaveTimer = null;
let advancedPreferencesVisible = localStorage.getItem('yt_archiver_advanced_preferences') === 'true';

// High-volume & pagination state
let inspectAbortController = null;
let selectedPlaylistIds = new Set();
let currentPlaylistPage = 1;
let playlistPageSize = 50;
let playlistSearchTerm = '';
let totalPlaylistPages = 1;
let currentQueuePage = 1;
let queuePageSize = 50;
let totalQueuePages = 1;
let currentLibraryPage = 1;
let libraryPageSize = 24;
let totalLibraryPages = 1;
let fetchDownloadsDebounceTimer = null;

// DOM Elements
const tabButtons = document.querySelectorAll('.nav-item');
const tabPanes = document.querySelectorAll('.tab-pane');
const targetUrlInput = document.getElementById('target-url-input');
const inspectBtn = document.getElementById('inspect-btn');
const previewCard = document.getElementById('preview-card');
const queueCardsContainer = document.getElementById('queue-cards-container');
const libraryGridContainer = document.getElementById('library-grid-container');
const channelsGridContainer = document.getElementById('channels-grid-container');
const activeQueueCountBadge = document.getElementById('active-queue-count');

// Init App
window.addEventListener('DOMContentLoaded', () => {
	const savedDensity = localStorage.getItem('yt_archiver_density');
	if (savedDensity) {
		applyUIMode(savedDensity);
	}
	applyTheme(localStorage.getItem('yt_archiver_theme') || 'midnight', localStorage.getItem('yt_archiver_color_scheme') || 'crimson');
	initLiquidGlassTracking();
	initSidebarState();
	setupTabs();
	updateQueueActionButtons(activeQueueFilter);
	enhanceAllSelects();
	initSSE();
	fetchDownloads();
	fetchChannels();
	loadPreferences();
	loadProfiles();
	const preferencesForm = document.getElementById('preferences-form');
	if (preferencesForm) {
		preferencesForm.addEventListener('input', queuePreferenceSave);
		preferencesForm.addEventListener('change', queuePreferenceSave);
	}
	setAdvancedPreferences(advancedPreferencesVisible);

	if (targetUrlInput) {
		targetUrlInput.addEventListener('keydown', (e) => {
			if (e.key === 'Enter') {
				inspectUrl();
			}
		});
	}

	const addChInput = document.getElementById('add-channel-input');
	if (addChInput) {
		addChInput.addEventListener('keydown', (e) => {
			if (e.key === 'Enter') {
				addChannel();
			}
		});
	}

	const prefNav = document.getElementById('pref-subnav');
	if (prefNav) {
		prefNav.addEventListener('wheel', (e) => {
			if (e.deltaY !== 0) {
				e.preventDefault();
				prefNav.scrollLeft += e.deltaY;
			}
		}, { passive: false });
	}

	// Ctrl+B shortcut to toggle sidebar
	window.addEventListener('keydown', (e) => {
		if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'b') {
			e.preventDefault();
			toggleSidebar();
		}
	});
});

// Collapsible Sidebar
function toggleSidebar() {
	const layout = document.querySelector('.app-layout');
	if (!layout) return;
	const isCollapsed = layout.classList.toggle('sidebar-collapsed');
	localStorage.setItem('yt_archiver_sidebar_collapsed', isCollapsed ? 'true' : 'false');
}

function initSidebarState() {
	const saved = localStorage.getItem('yt_archiver_sidebar_collapsed');
	const layout = document.querySelector('.app-layout');
	if (saved === 'true' && layout) {
		layout.classList.add('sidebar-collapsed');
	}
}

// Navigation Tabs
function setupTabs() {
	tabButtons.forEach(btn => {
		btn.addEventListener('click', () => {
			const target = btn.getAttribute('data-tab');
			switchTab(target);
		});
	});
}

function switchTab(tabId) {
	currentTab = tabId;
	tabButtons.forEach(b => {
		b.classList.toggle('active', b.getAttribute('data-tab') === tabId);
	});
	tabPanes.forEach(p => {
		const isTarget = p.id === tabId;
		p.classList.toggle('active', isTarget);
		if (isTarget) {
			// Re-trigger entrance animation
			p.style.animation = 'none';
			p.offsetHeight; // force reflow
			p.style.animation = '';
		}
	});

	const titles = {
		'download-tab': { title: 'New Download', desc: 'Extract videos, complete playlists, comments & metadata' },
		'queue-tab': { title: 'Downloads & Queue', desc: 'Live progress, speed, ETA & pause/resume controls' },
		'channels-tab': { title: 'Channels & Sync', desc: 'Subscribe to channels, track new uploads, and synchronize archives' },
		'library-tab': { title: 'Library & Archives', desc: 'Browse downloaded videos and launch interactive offline players' },
		'preferences-tab': { title: 'Preferences', desc: 'Configure cookies, SponsorBlock, format defaults & storage' }
	};

	if (titles[tabId]) {
		const titleEl = document.getElementById('current-view-title');
		const descEl = document.getElementById('current-view-desc');
		if (titleEl) {
			titleEl.textContent = titles[tabId].title;
			titleEl.classList.remove('view-header-animate');
			titleEl.offsetHeight;
			titleEl.classList.add('view-header-animate');
		}
		if (descEl) {
			descEl.textContent = titles[tabId].desc;
			descEl.classList.remove('view-header-animate');
			descEl.offsetHeight;
			descEl.classList.add('view-header-animate');
		}
	}

	if (tabId === 'queue-tab' || tabId === 'library-tab') {
		fetchDownloads();
	}
	if (tabId === 'channels-tab') {
		fetchChannels();
	}
}

// Download Mode (Single vs Batch)
function setDownloadMode(mode) {
	downloadMode = mode;
	const singleCard = document.getElementById('single-download-card');
	const batchCard = document.getElementById('batch-download-card');
	const singleBtn = document.getElementById('mode-single-btn');
	const batchBtn = document.getElementById('mode-batch-btn');

	if (mode === 'single') {
		singleCard.style.display = 'block';
		batchCard.style.display = 'none';
		singleBtn.classList.add('active');
		batchBtn.classList.remove('active');
	} else {
		singleCard.style.display = 'none';
		batchCard.style.display = 'block';
		singleBtn.classList.remove('active');
		batchBtn.classList.add('active');
	}
}

// Batch URLs Handlers
function handleBatchFileUpload(event) {
	const file = event.target.files[0];
	if (!file) return;

	const reader = new FileReader();
	reader.onload = (e) => {
		document.getElementById('batch-urls-textarea').value = e.target.result;
		showToast(`Loaded ${file.name}`, 'info');
	};
	reader.readAsText(file);
}

async function submitBatchDownload() {
	const rawText = document.getElementById('batch-urls-textarea').value;
	const lines = rawText.split('\n').map(l => l.trim()).filter(l => l && !l.startsWith('#'));

	if (lines.length === 0) {
		showToast('Please enter or upload at least one valid YouTube URL', 'error');
		return;
	}

	try {
		const res = await fetch('/api/download/batch', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ urls: lines })
		});

		const data = await res.json();
		if (!res.ok || data.error) {
			throw new Error(data.error || 'Failed to enqueue batch');
		}

		showToast(data.message || `Queued ${data.queued_count} items!`, 'success');
		document.getElementById('batch-urls-textarea').value = '';
		switchTab('queue-tab');
	} catch (err) {
		showToast(err.message, 'error');
	}
}

// Debounced fetchDownloads to prevent SSE request storms
function debouncedFetchDownloads(delay = 300) {
	if (fetchDownloadsDebounceTimer) clearTimeout(fetchDownloadsDebounceTimer);
	fetchDownloadsDebounceTimer = setTimeout(() => {
		fetchDownloads();
	}, delay);
}

// Server-Sent Events (SSE) & Real-Time Queue Sync
let queueSyncInterval = null;

function checkAndStartQueueSync() {
	const hasActive = allDownloads.some(d => d.status === 'downloading' || d.status === 'queued');
	if (hasActive && !queueSyncInterval) {
		queueSyncInterval = setInterval(() => {
			fetchDownloads();
		}, 1500);
	} else if (!hasActive && queueSyncInterval) {
		clearInterval(queueSyncInterval);
		queueSyncInterval = null;
	}
}

function initSSE() {
	if (eventSource) {
		try { eventSource.close(); } catch (e) {}
	}

	eventSource = new EventSource('/api/events');

	eventSource.onmessage = (e) => {
		try {
			const msg = JSON.parse(e.data);
			handleSSEEvent(msg);
		} catch (err) {
			console.error('Failed to parse SSE event', err);
		}
	};

	eventSource.addEventListener('progress', (e) => {
		try {
			const item = JSON.parse(e.data);
			updateItemProgress(item);
		} catch (err) {}
	});

	eventSource.addEventListener('status_change', (e) => {
		try {
			const item = JSON.parse(e.data);
			updateItemStatus(item);
		} catch (err) {}
	});

	eventSource.addEventListener('queue_update', () => {
		debouncedFetchDownloads(150);
	});

	eventSource.addEventListener('queue_batch_added', () => {
		fetchDownloads();
	});

	eventSource.addEventListener('toast', (e) => {
		try {
			const data = JSON.parse(e.data);
			if (data && data.message) {
				showToast(data.message, data.type || 'info');
			}
		} catch (err) {}
	});

	eventSource.addEventListener('circuit_breaker', (e) => {
		try {
			const data = JSON.parse(e.data);
			handleCircuitBreakerEvent(data);
		} catch (err) {}
	});

	eventSource.onerror = () => {
		setTimeout(initSSE, 3000);
	};
}

function handleSSEEvent(msg) {
	if (!msg) return;
	const data = msg.data || msg.Data;
	if (msg.type === 'progress') {
		if (data) updateItemProgress(data);
	} else if (msg.type === 'status_change') {
		if (data) updateItemStatus(data);
		debouncedFetchDownloads(200);
	} else if (msg.type === 'queue_update' || msg.type === 'queue_batch_added') {
		debouncedFetchDownloads(150);
	} else if (msg.type === 'toast') {
		if (data) showToast(data.message, data.type);
	} else if (msg.type === 'circuit_breaker') {
		if (data) handleCircuitBreakerEvent(data);
	}
}

function normalizeYouTubeInput(str) {
	str = (str || '').trim();
	if (!str) return '';
	if (str.startsWith('@')) {
		return 'https://www.youtube.com/' + str;
	}
	if (str.startsWith('UC') && str.length === 24 && !str.includes('/')) {
		return 'https://www.youtube.com/channel/' + str;
	}
	if (str.startsWith('youtube.com/') || str.startsWith('www.youtube.com/') || str.startsWith('m.youtube.com/') || str.startsWith('youtu.be/')) {
		return 'https://' + str;
	}
	if (!str.startsWith('http://') && !str.startsWith('https://')) {
		if (!str.includes('/') && !str.includes('.')) {
			return 'https://www.youtube.com/@' + str;
		}
		return 'https://' + str;
	}
	return str;
}

// Inspect URL with AbortController support
async function inspectUrl() {
	const raw = targetUrlInput.value.trim();
	if (!raw) {
		showToast('Please enter a YouTube video or playlist URL', 'error');
		return;
	}
	const url = normalizeYouTubeInput(raw);

	const btnText = inspectBtn.querySelector('.btn-text');
	const btnLoader = inspectBtn.querySelector('.btn-loader');
	const cancelBtn = document.getElementById('inspect-cancel-btn');

	btnText.style.display = 'none';
	btnLoader.style.display = 'inline-block';
	inspectBtn.disabled = true;
	if (cancelBtn) cancelBtn.style.display = 'inline-block';

	inspectAbortController = new AbortController();

	try {
		const res = await fetch('/api/info', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ url }),
			signal: inspectAbortController.signal
		});

		const data = await res.json();
		if (!res.ok || data.error) {
			throw new Error(data.error || 'Failed to inspect URL');
		}

		inspectedData = data;
		renderPreview(data);
		showToast('URL successfully inspected!', 'success');
	} catch (err) {
		if (err.name === 'AbortError') {
			showToast('Inspection cancelled', 'info');
		} else {
			showToast(err.message, 'error');
		}
	} finally {
		btnText.style.display = 'inline';
		btnLoader.style.display = 'none';
		inspectBtn.disabled = false;
		if (cancelBtn) cancelBtn.style.display = 'none';
		inspectAbortController = null;
	}
}

function cancelInspectUrl() {
	if (inspectAbortController) {
		inspectAbortController.abort();
	}
}

function renderPreview(data) {
	previewCard.style.display = 'block';

	document.getElementById('preview-thumb').src = data.thumbnail || 'data:image/svg+xml;utf8,<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360" fill="%23111"></svg>';
	document.getElementById('preview-duration').textContent = data.is_playlist ? `${data.item_count} Videos` : formatDuration(data.duration);
	
	const badge = document.getElementById('preview-type-badge');
	if (data.is_duplicate) {
		badge.textContent = 'ALREADY IN ARCHIVE';
		badge.style.background = 'var(--accent-warning, #f59e0b)';
		badge.style.color = '#000';
	} else {
		badge.textContent = data.is_playlist ? 'PLAYLIST' : 'VIDEO';
		badge.style.background = '';
		badge.style.color = '';
	}

	document.getElementById('preview-title').textContent = data.title;
	document.getElementById('preview-channel').textContent = data.channel || 'YouTube';
	document.getElementById('preview-item-count').textContent = data.is_duplicate ? (data.duplicate_reason || 'Already downloaded in library') : (data.is_playlist ? `${data.item_count} Items in playlist` : 'Single video stream');

	const type = document.getElementById('quick-type-select').value;
	const qual = type === 'audio' ? document.getElementById('quick-audio-fmt-select').value.toUpperCase() : document.getElementById('quick-quality-select').value;
	document.getElementById('preview-format-summary').textContent = `${type === 'audio' ? 'Audio Only' : 'Video'}: ${qual}`;

	const plBox = document.getElementById('playlist-items-box');
	if (data.is_playlist && data.items && data.items.length > 0) {
		plBox.style.display = 'block';
		selectedPlaylistIds = new Set(data.items.map(it => it.id));
		currentPlaylistPage = 1;
		playlistSearchTerm = '';
		const searchInput = document.getElementById('playlist-search-input');
		if (searchInput) searchInput.value = '';
		renderPlaylistPage();
	} else {
		plBox.style.display = 'none';
		selectedPlaylistIds = new Set();
	}
}

function handlePlaylistSearch(val) {
	playlistSearchTerm = (val || '').toLowerCase().trim();
	currentPlaylistPage = 1;
	renderPlaylistPage();
}

function changePlaylistPageSize(size) {
	playlistPageSize = parseInt(size, 10) || 50;
	currentPlaylistPage = 1;
	renderPlaylistPage();
}

function getFilteredPlaylistItems() {
	if (!inspectedData || !inspectedData.items) return [];
	if (!playlistSearchTerm) return inspectedData.items;
	return inspectedData.items.filter(it => (it.title || '').toLowerCase().includes(playlistSearchTerm) || (it.id || '').toLowerCase().includes(playlistSearchTerm));
}

function renderPlaylistPage() {
	const plList = document.getElementById('playlist-scroll-list');
	if (!plList || !inspectedData || !inspectedData.items) return;

	const filtered = getFilteredPlaylistItems();
	const total = filtered.length;
	totalPlaylistPages = Math.ceil(total / playlistPageSize) || 1;
	if (currentPlaylistPage > totalPlaylistPages) currentPlaylistPage = totalPlaylistPages;
	if (currentPlaylistPage < 1) currentPlaylistPage = 1;

	const startIdx = (currentPlaylistPage - 1) * playlistPageSize;
	const endIdx = Math.min(startIdx + playlistPageSize, total);
	const pageItems = filtered.slice(startIdx, endIdx);

	document.getElementById('total-playlist-items').textContent = inspectedData.items.length;
	updatePlaylistSelectionCount();

	plList.innerHTML = '';
	if (pageItems.length === 0) {
		plList.innerHTML = '<div style="padding: 24px; text-align: center; color: var(--text-secondary);">No matching videos found</div>';
	} else {
		pageItems.forEach((it, idx) => {
			const globalIndex = it.index || (startIdx + idx + 1);
			const row = document.createElement('div');
			row.className = 'playlist-item-row';
			const isChecked = selectedPlaylistIds.has(it.id);
			row.innerHTML = `
				<input type="checkbox" id="pl-item-${it.id}" class="pl-checkbox" value="${it.id}" ${isChecked ? 'checked' : ''} onchange="handlePlaylistItemCheckbox('${it.id}', this.checked)">
				<span class="pl-index">${globalIndex}</span>
				<div class="pl-title" title="${escapeHTML(it.title)}">${escapeHTML(it.title)}</div>
				<span class="pl-duration">${formatDuration(it.duration)}</span>
			`;
			plList.appendChild(row);
		});
	}

	// Update pagination controls
	const rangeEl = document.getElementById('playlist-page-range');
	if (rangeEl) rangeEl.textContent = total > 0 ? `Showing ${startIdx + 1}–${endIdx} of ${total}` : '0 items';
	const curPageEl = document.getElementById('pl-current-page-num');
	if (curPageEl) curPageEl.textContent = currentPlaylistPage;
	const totPageEl = document.getElementById('pl-total-pages-num');
	if (totPageEl) totPageEl.textContent = totalPlaylistPages;

	const btnFirst = document.getElementById('pl-btn-first');
	const btnPrev = document.getElementById('pl-btn-prev');
	const btnNext = document.getElementById('pl-btn-next');
	const btnLast = document.getElementById('pl-btn-last');

	if (btnFirst) btnFirst.disabled = currentPlaylistPage <= 1;
	if (btnPrev) btnPrev.disabled = currentPlaylistPage <= 1;
	if (btnNext) btnNext.disabled = currentPlaylistPage >= totalPlaylistPages;
	if (btnLast) btnLast.disabled = currentPlaylistPage >= totalPlaylistPages;
}

function handlePlaylistItemCheckbox(id, checked) {
	if (checked) {
		selectedPlaylistIds.add(id);
	} else {
		selectedPlaylistIds.delete(id);
	}
	updatePlaylistSelectionCount();
}

function updatePlaylistSelectionCount() {
	const countEl = document.getElementById('selected-items-count');
	if (countEl) countEl.textContent = selectedPlaylistIds.size;
}

function toggleSelectAllPlaylist(select) {
	if (!inspectedData || !inspectedData.items) return;
	const filtered = getFilteredPlaylistItems();
	if (select) {
		filtered.forEach(it => selectedPlaylistIds.add(it.id));
	} else {
		filtered.forEach(it => selectedPlaylistIds.delete(it.id));
	}
	renderPlaylistPage();
}

function selectFirstNPlaylist(n) {
	if (!inspectedData || !inspectedData.items) return;
	selectedPlaylistIds.clear();
	const count = Math.min(n, inspectedData.items.length);
	for (let i = 0; i < count; i++) {
		selectedPlaylistIds.add(inspectedData.items[i].id);
	}
	renderPlaylistPage();
}

function invertPlaylistSelection() {
	if (!inspectedData || !inspectedData.items) return;
	const filtered = getFilteredPlaylistItems();
	filtered.forEach(it => {
		if (selectedPlaylistIds.has(it.id)) {
			selectedPlaylistIds.delete(it.id);
		} else {
			selectedPlaylistIds.add(it.id);
		}
	});
	renderPlaylistPage();
}

function goToPlaylistPage(page) {
	if (page < 1 || page > totalPlaylistPages) return;
	currentPlaylistPage = page;
	renderPlaylistPage();
}

function jumpToPlaylistPage() {
	const input = document.getElementById('pl-jump-input');
	if (!input) return;
	const p = parseInt(input.value, 10);
	if (p >= 1 && p <= totalPlaylistPages) {
		goToPlaylistPage(p);
		input.value = '';
	}
}

function clearPreview() {
	inspectedData = null;
	selectedPlaylistIds.clear();
	previewCard.style.display = 'none';
	targetUrlInput.value = '';
}

function toggleQuickType(val) {
	const qWrapper = document.getElementById('quick-quality-wrapper');
	const aWrapper = document.getElementById('quick-audio-fmt-wrapper');
	if (val === 'audio') {
		qWrapper.style.display = 'none';
		aWrapper.style.display = 'flex';
	} else {
		qWrapper.style.display = 'flex';
		aWrapper.style.display = 'none';
	}
}

// Replace native select popovers with a consistent, keyboard-friendly control.
function enhanceAllSelects(root = document) {
	root.querySelectorAll('select').forEach(enhanceSelect);
}

function enhanceSelect(select) {
	if (!select || select.dataset.customized === 'true' || select.multiple) return;
	select.dataset.customized = 'true';
	const wrap = document.createElement('div');
	wrap.className = 'custom-select-wrap';
	select.parentNode.insertBefore(wrap, select);
	wrap.appendChild(select);
	select.classList.add('custom-select-native');
	select.tabIndex = -1;
	const trigger = document.createElement('button');
	trigger.type = 'button'; trigger.className = 'custom-select-trigger';
	trigger.setAttribute('aria-haspopup', 'listbox'); trigger.setAttribute('aria-expanded', 'false');
	const chevron = document.createElement('span'); chevron.className = 'custom-select-chevron'; chevron.innerHTML = '<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor"><path d="M7.41 8.59L12 13.17l4.59-4.58L18 10l-6 6-6-6 1.41-1.41z"/></svg>';
	const label = document.createElement('span'); label.className = 'custom-select-label';
	trigger.append(label, chevron); wrap.appendChild(trigger);
	const menu = document.createElement('div'); menu.className = 'custom-select-menu'; menu.setAttribute('role', 'listbox'); wrap.appendChild(menu);
	const sync = () => {
		const option = select.options[select.selectedIndex];
		label.textContent = option ? option.textContent : 'Choose an option';
		menu.querySelectorAll('[role="option"]').forEach(item => {
			const selected = item.dataset.value === select.value;
			item.classList.toggle('selected', selected);
			item.setAttribute('aria-selected', selected ? 'true' : 'false');
		});
	};
	const rebuild = () => {
		menu.innerHTML = '';
		Array.from(select.options).forEach(option => {
			const item = document.createElement('button'); item.type = 'button'; item.className = 'custom-select-option';
			item.setAttribute('role', 'option'); item.dataset.value = option.value; item.textContent = option.textContent;
			item.disabled = option.disabled;
			item.addEventListener('click', () => { select.value = option.value; select.dispatchEvent(new Event('change', { bubbles: true })); close(); sync(); });
			menu.appendChild(item);
		});
		sync();
	};
	const close = () => {
		wrap.classList.remove('open');
		wrap.closest('.url-input-card, .pref-card, .quick-options-row, .opt-chip, .form-group')?.classList.remove('select-open');
		trigger.setAttribute('aria-expanded', 'false');
	};
	const open = () => {
		document.querySelectorAll('.custom-select-wrap.open').forEach(other => {
			if (other !== wrap) {
				other.classList.remove('open');
				other.closest('.url-input-card, .pref-card, .quick-options-row, .opt-chip, .form-group')?.classList.remove('select-open');
			}
		});
		wrap.classList.add('open');
		wrap.closest('.url-input-card, .pref-card, .quick-options-row, .opt-chip, .form-group')?.classList.add('select-open');
		trigger.setAttribute('aria-expanded', 'true');
		const current = menu.querySelector('.selected');
		if (current) current.focus();
	};
	menu.addEventListener('keydown', event => {
		const options = [...menu.querySelectorAll('.custom-select-option:not(:disabled)')]; const current = options.indexOf(document.activeElement);
		if (event.key === 'ArrowDown' || event.key === 'ArrowUp') { event.preventDefault(); const next = event.key === 'ArrowDown' ? (current + 1) % options.length : (current - 1 + options.length) % options.length; options[next]?.focus(); }
		if (event.key === 'Escape') { event.preventDefault(); close(); trigger.focus(); }
	});
	trigger.addEventListener('click', () => wrap.classList.contains('open') ? close() : open());
	trigger.addEventListener('keydown', event => { if (['Enter', ' ', 'ArrowDown'].includes(event.key)) { event.preventDefault(); open(); } else if (event.key === 'Escape') close(); });
	select.addEventListener('change', sync);
	select._customSync = sync; select._customRebuild = rebuild;
	rebuild();
}

function syncCustomSelects() { document.querySelectorAll('select').forEach(select => select._customSync?.()); }
function refreshCustomSelect(select) { if (select?._customRebuild) select._customRebuild(); else enhanceSelect(select); }
document.addEventListener('click', event => {
	if (!event.target.closest('.custom-select-wrap')) {
		document.querySelectorAll('.custom-select-wrap.open').forEach(wrap => {
			wrap.classList.remove('open');
			wrap.closest('.url-input-card, .pref-card, .quick-options-row, .opt-chip, .form-group')?.classList.remove('select-open');
		});
	}
});

// Start Download
async function startDownload() {
	const url = targetUrlInput.value.trim();
	if (!url) return;

	const isAudio = document.getElementById('quick-type-select').value === 'audio';
	const videoQuality = document.getElementById('quick-quality-select').value;
	const audioFormat = document.getElementById('quick-audio-fmt-select').value;
	
	let commentLimit = 1000;
	const qCommVal = document.getElementById('quick-comments-select') ? document.getElementById('quick-comments-select').value : '1000';
	if (qCommVal === 'custom') {
		commentLimit = parseInt(document.getElementById('quick-comments-custom')?.value, 10) || 1000;
	} else {
		commentLimit = parseInt(qCommVal, 10);
	}

	const generateHTML = document.getElementById('quick-html-toggle').checked;

	let selectedIds = [];
	let itemsToSend = [];
	if (inspectedData && inspectedData.is_playlist && inspectedData.items) {
		if (selectedPlaylistIds.size === 0) {
			showToast('Please select at least 1 video from the playlist', 'error');
			return;
		}
		selectedIds = Array.from(selectedPlaylistIds);
		itemsToSend = inspectedData.items.filter(it => selectedPlaylistIds.has(it.id));
	} else if (inspectedData && inspectedData.items && inspectedData.items.length === 1) {
		itemsToSend = inspectedData.items;
	}

	const videoFormat = document.getElementById('pref-video-format') ? document.getElementById('pref-video-format').value : 'mp4';
	const profilePrefs = (downloadProfiles.find(p => p.id === document.getElementById('quick-profile-select').value) || {}).preferences || {};
	const customPrefs = Object.assign({}, profilePrefs, {
		download_video: !isAudio,
		video_format: videoFormat,
		video_quality: videoQuality,
		download_audio_only: isAudio,
		audio_format: audioFormat,
		download_thumbnail: true,
		download_metadata: true,
		download_comments: commentLimit > 0 || commentLimit === -1,
		comment_limit: commentLimit,
		download_commenter_avatars: true,
		generate_html_report: generateHTML
	});

	try {
		const res = await fetch('/api/download', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({
				url: url,
				playlist_title: (inspectedData && inspectedData.is_playlist && inspectedData.title) ? inspectedData.title : '',
				playlist_id: (inspectedData && inspectedData.is_playlist && inspectedData.playlist_id) ? inspectedData.playlist_id : '',
				channel_url: (inspectedData && inspectedData.channel_url) || '',
				selected_ids: selectedIds,
				items: itemsToSend,
				custom_prefs: customPrefs
			})
		});

		const data = await res.json();
		if (!res.ok || data.error) {
			throw new Error(data.error || 'Failed to start download');
		}

		if (data.queued_count === 0 && data.skipped_count > 0) {
			showToast(data.message || 'Already in your library! Skipped.', 'info');
		} else {
			showToast(data.message || `Queued ${data.queued_count} items!`, 'success');
		}
		clearPreview();
		switchTab('queue-tab');
		fetchDownloads();
	} catch (err) {
		showToast(err.message, 'error');
	}
}

// Queue & Downloads Management
async function fetchDownloads() {
	try {
		const res = await fetch('/api/downloads');
		if (!res.ok) return;
		allDownloads = await res.json() || [];
		renderQueue();
		renderLibrary();
		updateActiveCount();
		checkAndStartQueueSync();
	} catch (err) {
		console.error('Failed to fetch downloads', err);
	}
}

function updateActiveCount() {
	const active = allDownloads.filter(d => d.status === 'downloading' || d.status === 'queued').length;
	if (active > 0) {
		activeQueueCountBadge.style.display = 'inline-block';
		activeQueueCountBadge.textContent = active;
	} else {
		activeQueueCountBadge.style.display = 'none';
	}
}

function updateQueueActionButtons(filter) {
	const fetchMissingBtn = document.getElementById('fetch-missing-btn');
	const retryFailedBtn = document.getElementById('retry-failed-btn');
	const randomizeQueueBtn = document.getElementById('randomize-queue-btn');
	const revertQueueBtn = document.getElementById('revert-queue-btn');
	const pauseAllBtn = document.getElementById('pause-all-btn');
	const resumeAllBtn = document.getElementById('resume-all-btn');

	if (fetchMissingBtn) fetchMissingBtn.style.display = 'none';
	if (retryFailedBtn) retryFailedBtn.style.display = 'none';
	if (randomizeQueueBtn) randomizeQueueBtn.style.display = 'none';
	if (revertQueueBtn) revertQueueBtn.style.display = 'none';
	if (pauseAllBtn) pauseAllBtn.style.display = 'none';
	if (resumeAllBtn) resumeAllBtn.style.display = 'none';

	switch (filter) {
		case 'completed':
			if (fetchMissingBtn) fetchMissingBtn.style.display = 'inline-flex';
			break;
		case 'failed':
			if (retryFailedBtn) retryFailedBtn.style.display = 'inline-flex';
			break;
		case 'paused':
			if (resumeAllBtn) resumeAllBtn.style.display = 'inline-flex';
			break;
		case 'queued':
			if (pauseAllBtn) pauseAllBtn.style.display = 'inline-flex';
			if (randomizeQueueBtn) randomizeQueueBtn.style.display = 'inline-flex';
			if (revertQueueBtn && isQueueRandomized) revertQueueBtn.style.display = 'inline-flex';
			break;
		case 'downloading':
			if (pauseAllBtn) pauseAllBtn.style.display = 'inline-flex';
			break;
		case 'all':
		default:
			if (pauseAllBtn) pauseAllBtn.style.display = 'inline-flex';
			if (resumeAllBtn) resumeAllBtn.style.display = 'inline-flex';
			break;
	}
}

async function randomizeQueueOrder() {
	const queuedItems = allDownloads.filter(item => item.status === 'queued');
	if (queuedItems.length <= 1) {
		showToast('Not enough queued items to randomize.', 'info');
		return;
	}

	// Capture the original initial order before first randomization
	if (originalQueueOrder.length === 0) {
		const sortedInitial = [...queuedItems].sort((a, b) => {
			if (a.playlist_index && b.playlist_index) {
				return a.playlist_index - b.playlist_index;
			}
			return (new Date(a.created_at || 0)) - (new Date(b.created_at || 0));
		});
		originalQueueOrder = sortedInitial.map(item => item.id);
	}

	isQueueRandomized = true;
	// Shuffle using Fisher-Yates
	const shuffled = [...queuedItems];
	for (let i = shuffled.length - 1; i > 0; i--) {
		const j = Math.floor(Math.random() * (i + 1));
		[shuffled[i], shuffled[j]] = [shuffled[j], shuffled[i]];
	}
	randomizedQueueMap.clear();
	const shuffledIDs = shuffled.map((item, idx) => {
		randomizedQueueMap.set(item.id, idx);
		return item.id;
	});
	updateQueueActionButtons(activeQueueFilter);
	renderQueue();

	try {
		const res = await fetch('/api/queue/reorder', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ ids: shuffledIDs })
		});
		const data = await res.json();
		if (data.success) {
			showToast('Queue randomized and applied to downloader!', 'success');
		}
	} catch (e) {
		showToast('Queue randomized!', 'success');
	}
}

async function revertQueueOrder() {
	const queuedItems = allDownloads.filter(item => item.status === 'queued');
	isQueueRandomized = false;
	randomizedQueueMap.clear();

	let revertIDs = [];
	if (originalQueueOrder.length > 0) {
		const currentSet = new Set(queuedItems.map(i => i.id));
		revertIDs = originalQueueOrder.filter(id => currentSet.has(id));
		// Append any newly added items not in the original snapshot
		queuedItems.forEach(item => {
			if (!revertIDs.includes(item.id)) {
				revertIDs.push(item.id);
			}
		});
	} else {
		// Fallback: sort by playlist_index or created_at
		const original = [...queuedItems].sort((a, b) => {
			if (a.playlist_index && b.playlist_index) {
				return a.playlist_index - b.playlist_index;
			}
			return (new Date(a.created_at || 0)) - (new Date(b.created_at || 0));
		});
		revertIDs = original.map(item => item.id);
	}

	originalQueueOrder = []; // Reset after reverting
	updateQueueActionButtons(activeQueueFilter);
	renderQueue();

	try {
		const res = await fetch('/api/queue/reorder', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ ids: revertIDs })
		});
		const data = await res.json();
		if (data.success) {
			showToast('Queue reverted to original added order.', 'info');
		}
	} catch (e) {
		showToast('Queue order reverted to original added order.', 'info');
	}
}

function filterQueue(status) {
	activeQueueFilter = status;
	currentQueuePage = 1;
	document.querySelectorAll('.queue-filter-tabs .filter-pill').forEach(btn => {
		btn.classList.toggle('active', btn.textContent.toLowerCase() === status);
	});
	updateQueueActionButtons(status);
	renderQueue();
}

function renderQueue() {
	queueCardsContainer.innerHTML = '';

	let filtered = allDownloads.filter(item => {
		if (activeQueueFilter === 'all') return true;
		return item.status === activeQueueFilter;
	});

	if (activeQueueFilter === 'queued' && isQueueRandomized && randomizedQueueMap.size > 0) {
		filtered = [...filtered].sort((a, b) => {
			const orderA = randomizedQueueMap.has(a.id) ? randomizedQueueMap.get(a.id) : 999999;
			const orderB = randomizedQueueMap.has(b.id) ? randomizedQueueMap.get(b.id) : 999999;
			return orderA - orderB;
		});
	}

	const paginationBar = document.getElementById('queue-pagination-bar');

	if (filtered.length === 0) {
		queueCardsContainer.innerHTML = `
			<div class="empty-state">
				<svg viewBox="0 0 24 24"><path d="M19 9h-4V3H9v6H5l7 7 7-7zM5 18v2h14v-2H5z"/></svg>
				<h3>No Downloads in Queue</h3>
				<p>Paste a YouTube video or playlist URL in the New Download tab to begin.</p>
			</div>
		`;
		if (paginationBar) paginationBar.style.display = 'none';
		return;
	}

	const total = filtered.length;
	totalQueuePages = Math.ceil(total / queuePageSize) || 1;
	if (currentQueuePage > totalQueuePages) currentQueuePage = totalQueuePages;
	if (currentQueuePage < 1) currentQueuePage = 1;

	const startIdx = (currentQueuePage - 1) * queuePageSize;
	const endIdx = Math.min(startIdx + queuePageSize, total);
	const pageItems = filtered.slice(startIdx, endIdx);

	pageItems.forEach(item => {
		const card = createDownloadCard(item);
		queueCardsContainer.appendChild(card);
	});

	if (paginationBar) {
		if (total > queuePageSize) {
			paginationBar.style.display = 'flex';
			document.getElementById('queue-page-range').textContent = `Showing ${startIdx + 1}–${endIdx} of ${total} items`;
			document.getElementById('q-current-page-num').textContent = currentQueuePage;
			document.getElementById('q-total-pages-num').textContent = totalQueuePages;
			document.getElementById('q-btn-first').disabled = currentQueuePage <= 1;
			document.getElementById('q-btn-prev').disabled = currentQueuePage <= 1;
			document.getElementById('q-btn-next').disabled = currentQueuePage >= totalQueuePages;
			document.getElementById('q-btn-last').disabled = currentQueuePage >= totalQueuePages;
		} else {
			paginationBar.style.display = 'none';
		}
	}
}

function goToQueuePage(page) {
	if (page < 1 || page > totalQueuePages) return;
	currentQueuePage = page;
	renderQueue();
	window.scrollTo({ top: 0, behavior: 'smooth' });
}

function createDownloadCard(item) {
	const card = document.createElement('div');
	card.className = `download-card ${item.status === 'downloading' ? 'active-downloading' : ''}`;
	card.id = `card-${item.id}`;

	let actionButtons = '';
	if (item.status === 'downloading') {
		actionButtons = `
			<button class="btn btn-sm btn-secondary" onclick="pauseDownload('${item.id}')">Pause</button>
			<button class="btn btn-sm btn-danger" onclick="cancelDownload('${item.id}')">Cancel</button>
		`;
	} else if (item.status === 'paused') {
		actionButtons = `
			<button class="btn btn-sm btn-primary" onclick="resumeDownload('${item.id}')">Resume</button>
			<button class="btn btn-sm btn-danger" onclick="cancelDownload('${item.id}')">Cancel</button>
		`;
	} else if (item.status === 'failed') {
		actionButtons = `
			<button class="btn btn-sm btn-primary" onclick="retryDownload('${item.id}')">Retry</button>
			<button class="btn btn-sm btn-secondary" onclick="retryWithAltClient('${item.id}')" title="Retry with alternative player clients (android_vr/ios/mweb) and fallback stream">Alt Client</button>
			<button class="btn btn-sm btn-danger" onclick="deleteDownload('${item.id}', false)">Remove</button>
		`;
	} else if (item.status === 'cancelled') {
		actionButtons = `
			<button class="btn btn-sm btn-secondary" onclick="retryDownload('${item.id}')">Retry</button>
			<button class="btn btn-sm btn-danger" onclick="deleteDownload('${item.id}', false)">Remove</button>
		`;
	} else if (item.status === 'completed') {
		actionButtons = `
			<button class="btn btn-sm btn-primary" onclick="openHTMLPlayer('${item.id}')">Watch Offline</button>
			<button class="btn btn-sm btn-secondary" onclick="openFolder('${item.id}')">Open Folder</button>
			<button class="btn btn-sm btn-danger" onclick="deleteDownload('${item.id}', true)">Delete</button>
		`;
	}

	card.innerHTML = `
		<div class="download-card-thumb">
			<img src="${item.thumbnail_url || ''}" alt="Thumbnail" onerror="this.src='data:image/svg+xml;utf8,<svg xmlns=\\'http://www.w3.org/2000/svg\\' width=\\'180\\' height=\\'100\\' fill=\\'%23111\\'></svg>'"/>
		</div>
		<div class="download-card-content">
			<div>
				<div class="card-title-row">
					<span class="card-title" title="${escapeHTML(item.title)}">${escapeHTML(item.title)}</span>
					<span class="status-badge ${item.status}">${item.status}</span>
				</div>
				<div class="card-meta-row">
					<span>${escapeHTML(item.channel || 'YouTube')}</span>
					<span class="bullet">&bull;</span>
					<span>${item.is_audio_only ? 'Audio (' + item.format + ')' : item.quality}</span>
					${item.playlist_title ? `<span class="bullet">&bull;</span><span>${escapeHTML(item.playlist_title)} (#${item.playlist_index})</span>` : ''}
				</div>
			</div>

			<div class="progress-container">
				<div class="progress-bar-track">
					<div class="progress-bar-fill" id="pfill-${item.id}" style="width: ${item.progress || 0}%;"></div>
				</div>
				<div class="progress-stats-row">
					<span class="step-label" id="pstep-${item.id}">${item.current_step || item.status}</span>
					<span id="pstats-${item.id}">
						${item.progress > 0 ? item.progress.toFixed(1) + '%' : ''} 
						${item.speed ? '&bull; ' + item.speed : ''} 
						${item.eta ? '&bull; ETA: ' + item.eta : ''}
					</span>
				</div>
			</div>

			<div class="card-actions-row">
				${actionButtons}
			</div>
		</div>
	`;
	return card;
}

let circuitBreakerTimerInterval = null;

function handleCircuitBreakerEvent(data) {
	const banner = document.getElementById('circuit-breaker-banner');
	const msg = document.getElementById('circuit-breaker-message');
	const timer = document.getElementById('circuit-breaker-timer');
	if (!banner) return;

	if (circuitBreakerTimerInterval) {
		clearInterval(circuitBreakerTimerInterval);
		circuitBreakerTimerInterval = null;
	}

	if (data && data.active && data.seconds_remaining > 0) {
		banner.style.display = 'flex';
		if (msg && data.reason) {
			msg.textContent = data.reason;
		}
		let remaining = data.seconds_remaining;
		const updateTimer = () => {
			const m = Math.floor(remaining / 60);
			const s = remaining % 60;
			if (timer) timer.textContent = `Auto-resuming in: ${m}:${s.toString().padStart(2, '0')}`;
			if (remaining <= 0) {
				banner.style.display = 'none';
				clearInterval(circuitBreakerTimerInterval);
				fetchDownloads();
			}
			remaining--;
		};
		updateTimer();
		circuitBreakerTimerInterval = setInterval(updateTimer, 1000);
	} else {
		banner.style.display = 'none';
	}
}

async function resetCircuitBreaker() {
	try {
		await fetch('/api/queue/circuit-breaker/reset', { method: 'POST' });
		const banner = document.getElementById('circuit-breaker-banner');
		if (banner) banner.style.display = 'none';
		if (circuitBreakerTimerInterval) {
			clearInterval(circuitBreakerTimerInterval);
			circuitBreakerTimerInterval = null;
		}
		showToast('Rate limit cooldown cleared. Queue resumed.', 'success');
		fetchDownloads();
	} catch (err) {
		showToast('Failed to reset circuit breaker: ' + err.message, 'error');
	}
}

function updateItemProgress(item) {
	if (!item || !item.id) return;
	const fill = document.getElementById(`pfill-${item.id}`);
	const step = document.getElementById(`pstep-${item.id}`);
	const stats = document.getElementById(`pstats-${item.id}`);

	if (fill) fill.style.width = `${Math.min(100, Math.max(0, item.progress || 0))}%`;
	if (step) step.textContent = item.current_step || item.status || '';
	if (stats) {
		const parts = [];
		if (item.progress > 0) parts.push(`${item.progress.toFixed(1)}%`);
		if (item.speed) parts.push(item.speed);
		if (item.eta) parts.push(`ETA: ${item.eta}`);
		stats.innerHTML = parts.join(' &bull; ');
	}

	// Also update in-memory item
	const idx = allDownloads.findIndex(d => d.id === item.id);
	if (idx !== -1) {
		allDownloads[idx].progress = item.progress;
		allDownloads[idx].speed = item.speed;
		allDownloads[idx].eta = item.eta;
		allDownloads[idx].current_step = item.current_step;
		if (item.status) allDownloads[idx].status = item.status;
	} else {
		// New item not yet rendered on screen: fetch downloads
		debouncedFetchDownloads(200);
	}
	checkAndStartQueueSync();
}

function updateItemStatus(item) {
	fetchDownloads();
}

// Download Controls
async function pauseDownload(id) {
	await fetch(`/api/downloads/${id}/pause`, { method: 'POST' });
	fetchDownloads();
}

async function resumeDownload(id) {
	await fetch(`/api/downloads/${id}/resume`, { method: 'POST' });
	fetchDownloads();
}

async function cancelDownload(id) {
	await fetch(`/api/downloads/${id}/cancel`, { method: 'POST' });
	fetchDownloads();
}

async function retryDownload(id) {
	await fetch(`/api/downloads/${id}/retry`, { method: 'POST' });
	fetchDownloads();
}

async function retryWithAltClient(id) {
	try {
		const res = await fetch(`/api/downloads/${id}/retry-alternative`, { method: 'POST' });
		if (!res.ok) throw new Error('Failed to queue with alternative client');
		showToast('Re-queued with alternative player client & stream fallback', 'success');
		fetchDownloads();
	} catch (err) {
		showToast(err.message, 'error');
	}
}

async function retryAllFailedDownloads() {
	try {
		const res = await fetch('/api/queue/retry-all-failed', { method: 'POST' });
		const data = await res.json();
		if (!res.ok) throw new Error(data.error || 'Failed to retry failed downloads');
		showToast(data.message || 'Re-queued all failed downloads', 'success');
		fetchDownloads();
	} catch (err) {
		showToast(err.message, 'error');
	}
}

async function fetchMissingAssets() {
	const btn = document.getElementById('fetch-missing-btn');
	if (btn) btn.disabled = true;
	try {
		showToast('Scanning completed downloads for missing comments/assets...', 'info');
		const res = await fetch('/api/queue/fetch-missing', { method: 'POST' });
		const data = await res.json();
		if (!res.ok) throw new Error(data.error || 'Failed to scan missing assets');
		
		if (data.queued > 0) {
			showToast(data.message || `Queued ${data.queued} items to fetch missing assets!`, 'success');
		} else {
			showToast(data.message || 'All completed downloads are already fully archived!', 'info');
		}
		fetchDownloads();
	} catch (err) {
		showToast(err.message, 'error');
	} finally {
		if (btn) btn.disabled = false;
	}
}

function applyUIMode(mode) {
	if (mode === 'compact') {
		document.documentElement.setAttribute('data-density', 'compact');
		localStorage.setItem('yt_archiver_density', 'compact');
	} else {
		document.documentElement.removeAttribute('data-density');
		localStorage.setItem('yt_archiver_density', 'standard');
	}
	const sel = document.getElementById('pref-ui-mode');
	if (sel) sel.value = mode || 'standard';
}

function applyTheme(theme, colorScheme) {
	const safeTheme = ['midnight', 'liquid-glass', 'aurora', 'paper'].includes(theme) ? theme : 'midnight';
	const safeAccent = ['crimson', 'ocean', 'violet', 'lime', 'sunset', 'rose', 'teal', 'indigo', 'amber', 'slate'].includes(colorScheme) ? colorScheme : 'crimson';
	document.documentElement.setAttribute('data-theme', safeTheme);
	document.documentElement.setAttribute('data-accent', safeAccent);
	localStorage.setItem('yt_archiver_theme', safeTheme);
	localStorage.setItem('yt_archiver_color_scheme', safeAccent);
	const themeInput = document.getElementById('pref-theme'); if (themeInput) themeInput.value = safeTheme;
	const accentInput = document.getElementById('pref-color-scheme'); if (accentInput) accentInput.value = safeAccent;
	document.querySelectorAll('[data-theme-choice]').forEach(el => el.classList.toggle('selected', el.dataset.themeChoice === safeTheme));
	document.querySelectorAll('[data-accent-choice]').forEach(el => el.classList.toggle('selected', el.dataset.accentChoice === safeAccent));
}

function setTheme(theme) {
	applyTheme(theme, document.getElementById('pref-color-scheme')?.value || 'crimson');
	queuePreferenceSave();
}

function setColorScheme(scheme) {
	applyTheme(document.getElementById('pref-theme')?.value || 'midnight', scheme);
	queuePreferenceSave();
}

let liquidGlassTrackingInitialized = false;

function initLiquidGlassTracking() {
	if (liquidGlassTrackingInitialized) return;
	liquidGlassTrackingInitialized = true;

	let rafPending = false;
	let lastEvent = null;

	const glassSelector = 
		'.glass-surface, .pref-card, .overview-card, .url-input-card, .preview-card, ' +
		'.queue-card, .library-card, .channel-card, .feature-card, .empty-state, .download-card, ' +
		'.batch-url-card, .channel-add-card, .preferences-hero, .pref-subnav, .pref-subnav-pill, ' +
		'.queue-filter-tabs, .studio-filter-tabs, .filter-pill, .library-search-box, ' +
		'.nav-item, .btn-secondary, .btn-primary, .btn-danger, .opt-chip, .sidebar-toggle-btn, ' +
		'.custom-select-trigger, .toast, .modal-card, .studio-modal-content, .studio-toolbar, .top-bar, .sidebar';

	function updateGlassHighlight() {
		rafPending = false;
		if (!lastEvent) return;
		if (document.documentElement.getAttribute('data-theme') !== 'liquid-glass') return;

		const target = lastEvent.target.closest(glassSelector);
		if (target) {
			const rect = target.getBoundingClientRect();
			const x = Math.round(lastEvent.clientX - rect.left);
			const y = Math.round(lastEvent.clientY - rect.top);
			target.style.setProperty('--glass-x', `${x}px`);
			target.style.setProperty('--glass-y', `${y}px`);
		}
	}

	document.addEventListener('pointermove', (e) => {
		if (document.documentElement.getAttribute('data-theme') !== 'liquid-glass') return;
		lastEvent = e;
		if (!rafPending) {
			rafPending = true;
			requestAnimationFrame(updateGlassHighlight);
		}
	}, { passive: true });
}

async function deleteDownload(id, deleteFiles) {
	await fetch(`/api/downloads/${id}?delete_files=${deleteFiles}`, { method: 'DELETE' });
	fetchDownloads();
}

async function pauseAllDownloads() {
	await fetch('/api/queue/pause-all', { method: 'POST' });
	fetchDownloads();
}

async function resumeAllDownloads() {
	await fetch('/api/queue/resume-all', { method: 'POST' });
	fetchDownloads();
}

async function clearQueue() {
	const count = allDownloads.filter(d => d.status !== 'completed').length;
	if (count === 0) {
		showToast('Queue is already empty', 'info');
		return;
	}

	const confirmed = await showConfirmDialog({
		title: 'Clear Download Queue?',
		message: `Are you sure you want to cancel and remove ${count} pending/downloading item(s) from the queue? Completed archives will remain intact.`,
		confirmText: 'Clear Queue',
		cancelText: 'Keep Queue',
		danger: true
	});
	if (!confirmed) return;

	try {
		const res = await fetch('/api/queue/clear', { method: 'POST' });
		const data = await res.json();
		if (!res.ok || data.error) throw new Error(data.error || 'Failed to clear queue');
		showToast(data.message || `Cleared ${data.count || count} items from queue`, 'success');
		fetchDownloads();
	} catch (err) {
		showToast(err.message, 'error');
	}
}

async function openFolder(id) {
	if (id) {
		await fetch(`/api/downloads/${id}/open-folder`, { method: 'POST' });
	} else {
		const res = await fetch('/api/preferences');
		const prefs = await res.json();
		if (prefs && prefs.download_folder) {
			fetch('/api/downloads/default/open-folder', { method: 'POST' });
		}
	}
}

async function openHTMLPlayer(id) {
	const res = await fetch(`/api/downloads/${id}/open-html`, { method: 'POST' });
	if (!res.ok) {
		showToast('HTML Player file not found for this item', 'error');
	}
}

// Channels Tab Operations (Feature 2)
async function fetchChannels() {
	try {
		const res = await fetch('/api/channels');
		if (!res.ok) return;
		allChannels = await res.json() || [];
		renderChannels();
	} catch (err) {
		console.error('Failed to fetch channels', err);
	}
}

function formatSubscribers(num) {
	if (!num || num <= 0) return '';
	if (num >= 1000000) {
		const m = (num / 1000000).toFixed(2).replace(/\.00$/, '').replace(/(\.[0-9])0$/, '$1');
		return `${m}M subscribers`;
	}
	if (num >= 1000) {
		const k = (num / 1000).toFixed(1).replace(/\.0$/, '');
		return `${k}K subscribers`;
	}
	return `${num} subscribers`;
}

function renderChannels() {
	if (!channelsGridContainer) return;
	channelsGridContainer.innerHTML = '';
	const countEl = document.getElementById('channels-count-label');
	if (countEl) countEl.textContent = `${allChannels.length} Subscribed Channels`;

	if (allChannels.length === 0) {
		channelsGridContainer.innerHTML = `
			<div class="empty-state" style="grid-column: 1 / -1;">
				<svg viewBox="0 0 24 24"><path d="M21 3H3c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h5v2h8v-2h5c1.1 0 1.99-.9 1.99-2L23 5c0-1.1-.9-2-2-2zm0 14H3V5h18v12zm-5-6l-7 4V7l7 4z"/></svg>
				<h3>No Channels Subscribed</h3>
				<p>Add a channel above to track its catalog and download new releases automatically.</p>
			</div>
		`;
		return;
	}

	allChannels.forEach(c => {
		const card = document.createElement('div');
		card.className = 'channel-card';
		const channelName = c.title || c.name || 'YouTube Channel';
		const initial = (channelName.charAt(0) || 'C').toUpperCase();
		const cleanHandle = (c.handle || '').replace(/^@+/, '');
		const handleText = cleanHandle ? `@${cleanHandle}` : '';
		const subsText = formatSubscribers(c.subscriber_count);
		const videoCount = c.total_videos || 0;
		const videoText = videoCount > 0 ? `${videoCount} Videos` : 'Ready to sync';

		const metaParts = [];
		if (handleText) metaParts.push(escapeHTML(handleText));
		if (subsText) metaParts.push(escapeHTML(subsText));
		metaParts.push(escapeHTML(videoText));

		const avatarHTML = c.avatar_url ? `
			<img src="${escapeHTML(c.avatar_url)}" alt="${escapeHTML(channelName)}" class="channel-card-avatar" onerror="this.outerHTML='<div class=\\'channel-card-avatar-fallback\\'>${initial}</div>'"/>
		` : `<div class="channel-card-avatar-fallback">${initial}</div>`;

		card.innerHTML = `
			${avatarHTML}
			<div class="channel-card-info">
				<div class="channel-card-title" title="${escapeHTML(channelName)}">${escapeHTML(channelName)}</div>
				<div class="channel-card-meta">
					${metaParts.join(' &bull; ')}
				</div>
				<div class="channel-card-actions">
					<button class="btn btn-sm btn-secondary" onclick="openChannelStudio('${c.id}')" title="Open Channel Studio & Video Catalog">
						<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor" style="vertical-align:text-top;margin-right:4px;"><path d="M4 6H2v14c0 1.1.9 2 2 2h14v-2H4V6zm16-4H8c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm0 14H8V4h12v12z"/></svg>Studio
					</button>
					<button class="btn btn-sm btn-primary" onclick="syncChannel('${c.id}')">
						<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor" style="vertical-align:text-top;margin-right:5px;"><path d="M12 4V1L8 5l4 4V6c3.31 0 6 2.69 6 6 0 1.01-.25 1.97-.7 2.8l1.46 1.46C19.54 15.03 20 13.57 20 12c0-4.42-3.58-8-8-8zm0 14c-3.31 0-6-2.69-6-6 0-1.01.25-1.97.7-2.8L5.24 7.74C4.46 8.97 4 10.43 4 12c0 4.42 3.58 8 8 8v3l4-4-4-4v3z"/></svg>Sync Now
					</button>
					<button class="btn btn-sm btn-danger" onclick="deleteChannel('${c.id}')" title="Unsubscribe">
						<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor"><path d="M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/></svg>
					</button>
				</div>
			</div>
		`;
		channelsGridContainer.appendChild(card);
	});
}

async function addChannel() {
	const input = document.getElementById('add-channel-input');
	if (!input) return;
	const raw = input.value.trim();
	if (!raw) {
		showToast('Please enter a channel URL or handle (e.g. @CasuallyExplained)', 'error');
		return;
	}
	const url = normalizeYouTubeInput(raw);

	const btn = document.getElementById('add-channel-btn');
	if (btn) {
		btn.disabled = true;
		const btnText = btn.querySelector('.btn-text');
		if (btnText) btnText.textContent = 'Subscribing...';
	}

	try {
		const res = await fetch('/api/channels/add', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ url })
		});
		const data = await res.json();
		if (!res.ok || data.error) {
			throw new Error(data.error || 'Failed to add channel');
		}
		showToast(`Subscribed to ${data.title || data.channel?.name || 'channel'}!`, 'success');
		input.value = '';
		fetchChannels();
	} catch (err) {
		showToast(err.message, 'error');
	} finally {
		if (btn) {
			btn.disabled = false;
			const btnText = btn.querySelector('.btn-text');
			if (btnText) {
				btnText.textContent = 'Add Channel';
			} else {
				btn.textContent = 'Add Channel';
			}
		}
	}
}

async function syncChannel(id) {
	showToast('Checking channel for new videos…', 'info');
	try {
		const res = await fetch(`/api/channels/${id}/sync`, { method: 'POST' });
		const data = await res.json();
		if (!res.ok || data.error) {
			throw new Error(data.error || 'Failed to sync channel');
		}
		showToast(data.message || 'Channel synced successfully!', 'success');
		fetchChannels();
		fetchDownloads();
	} catch (err) {
		showToast(err.message, 'error');
	}
}

async function deleteChannel(id) {
	const confirmed = await showConfirmDialog({
		title: 'Unsubscribe Channel?',
		message: 'Are you sure you want to remove this channel subscription and stop tracking new uploads?',
		confirmText: 'Unsubscribe',
		cancelText: 'Keep Channel',
		danger: true
	});
	if (!confirmed) return;

	try {
		await fetch(`/api/channels/${id}`, { method: 'DELETE' });
		showToast('Channel removed', 'info');
		fetchChannels();
	} catch (err) {
		showToast(err.message, 'error');
	}
}

// ==========================================================================
// CHANNEL STUDIO & SELECTIVE ARCHIVING
// ==========================================================================
let currentStudioChannelId = null;
let currentStudioCatalog = [];
let currentStudioChannelData = null;
let studioCategoryFilter = 'all';
let studioStatusFilter = 'all';
let studioSelectedVideos = new Set();
let studioCurrentPage = 1;
let studioPageSize = 24;
let studioTotalPages = 1;

async function openChannelStudio(channelId) {
	currentStudioChannelId = channelId;
	currentStudioCatalog = [];
	studioSelectedVideos.clear();
	studioCategoryFilter = 'all';
	studioStatusFilter = 'all';
	studioCurrentPage = 1;

	const modal = document.getElementById('channel-studio-modal');
	if (!modal) return;
	modal.style.display = 'flex';

	// Reset tab
	switchStudioTab('catalog');

	// Reset search input
	const searchInput = document.getElementById('studio-search-input');
	if (searchInput) searchInput.value = '';

	// Reset filter pills
	resetStudioFilterPills();

	const container = document.getElementById('studio-catalog-container');
	if (container) {
		container.innerHTML = `
			<div class="studio-loading">
				<div class="btn-loader"></div>
				<span>Loading channel catalog & checking archived status...</span>
			</div>
		`;
	}

	try {
		const res = await fetch(`/api/channels/${channelId}/catalog`);
		const data = await res.json();
		if (!res.ok || data.error) {
			throw new Error(data.error || 'Failed to load channel catalog');
		}

		currentStudioChannelData = data.channel;
		currentStudioCatalog = data.videos || [];
		studioCurrentPage = 1;

		// Render header profile
		renderStudioHeader(data.channel, data.total_videos, data.archived_count);

		// Populate rules form
		populateStudioRules(data.channel);

		// Render catalog
		renderStudioCatalog();
	} catch (err) {
		if (container) {
			container.innerHTML = `
				<div class="studio-empty">
					<p style="color: var(--accent-danger);">Failed to load channel: ${escapeHTML(err.message)}</p>
					<button class="btn btn-sm btn-secondary" onclick="openChannelStudio('${channelId}')" style="margin-top: 10px;">Retry</button>
				</div>
			`;
		}
		showToast(err.message, 'error');
	}
}

function closeChannelStudio() {
	const modal = document.getElementById('channel-studio-modal');
	if (modal) modal.style.display = 'none';
	currentStudioChannelId = null;
	currentStudioCatalog = [];
	studioSelectedVideos.clear();
}

function handleStudioBackdropClick(event) {
	if (event.target && event.target.id === 'channel-studio-modal') {
		closeChannelStudio();
	}
}

function switchStudioTab(tabName) {
	const btnCatalog = document.getElementById('btn-studio-tab-catalog');
	const btnRules = document.getElementById('btn-studio-tab-rules');
	const paneCatalog = document.getElementById('studio-pane-catalog');
	const paneRules = document.getElementById('studio-pane-rules');

	const activePane = tabName === 'rules' ? paneRules : paneCatalog;

	if (tabName === 'rules') {
		if (btnCatalog) btnCatalog.classList.remove('active');
		if (btnRules) btnRules.classList.add('active');
		if (paneCatalog) paneCatalog.classList.remove('active');
		if (paneRules) paneRules.classList.add('active');
	} else {
		if (btnCatalog) btnCatalog.classList.add('active');
		if (btnRules) btnRules.classList.remove('active');
		if (paneCatalog) paneCatalog.classList.add('active');
		if (paneRules) paneRules.classList.remove('active');
	}

	// Re-trigger entrance animation
	if (activePane) {
		activePane.style.animation = 'none';
		activePane.offsetHeight;
		activePane.style.animation = '';
	}
}

function renderStudioHeader(channel, totalCount, archivedCount) {
	const titleEl = document.getElementById('studio-channel-title');
	const handleEl = document.getElementById('studio-channel-handle');
	const avatarBox = document.getElementById('studio-avatar-box');
	const metaEl = document.getElementById('studio-channel-meta');

	const title = channel?.title || 'YouTube Channel';
	const cleanHandle = (channel?.handle || '').replace(/^@+/, '');
	const initial = (title.charAt(0) || 'C').toUpperCase();

	if (titleEl) titleEl.textContent = title;
	if (handleEl) {
		if (cleanHandle) {
			handleEl.textContent = `@${cleanHandle}`;
			handleEl.style.display = 'inline-block';
		} else {
			handleEl.style.display = 'none';
		}
	}

	if (avatarBox) {
		if (channel?.avatar_url) {
			avatarBox.innerHTML = `<img src="${escapeHTML(channel.avatar_url)}" alt="${escapeHTML(title)}" onerror="this.outerHTML='<div class=\\'studio-avatar-fallback\\'>${initial}</div>'"/>`;
		} else {
			avatarBox.innerHTML = `<div class="studio-avatar-fallback">${initial}</div>`;
		}
	}

	if (metaEl) {
		const subs = formatSubscribers(channel?.subscriber_count);
		const total = totalCount || currentStudioCatalog.length || 0;
		const archived = archivedCount || 0;
		metaEl.innerHTML = `
			${subs ? `<span class="studio-meta-pill">${escapeHTML(subs)}</span>` : ''}
			<span class="studio-meta-pill">${total} Videos in Catalog</span>
			<span class="studio-meta-pill archived-count">${archived} Archived Locally</span>
		`;
	}
}

function populateStudioRules(channel) {
	if (!channel) return;
	const autoDl = document.getElementById('rule-auto-download');
	const exShorts = document.getElementById('rule-exclude-shorts');
	const exLive = document.getElementById('rule-exclude-live');
	const minDur = document.getElementById('rule-min-duration');
	const maxSync = document.getElementById('rule-max-auto-sync');

	if (autoDl) autoDl.checked = !!channel.auto_download;
	if (exShorts) exShorts.checked = !!channel.exclude_shorts;
	if (exLive) exLive.checked = !!channel.exclude_livestreams;
	if (minDur) minDur.value = channel.min_duration_sec || 0;
	if (maxSync) maxSync.value = channel.max_auto_sync_count || 0;
}

function resetStudioFilterPills() {
	document.querySelectorAll('#studio-category-filters .filter-pill').forEach(p => p.classList.remove('active'));
	document.querySelectorAll('#studio-status-filters .filter-pill').forEach(p => p.classList.remove('active'));
	const defaultCat = document.getElementById('cat-pill-all');
	const defaultStat = document.getElementById('stat-pill-all');
	if (defaultCat) defaultCat.classList.add('active');
	if (defaultStat) defaultStat.classList.add('active');
}

function setStudioCategoryFilter(category) {
	studioCategoryFilter = category;
	studioCurrentPage = 1;
	document.querySelectorAll('#studio-category-filters .filter-pill').forEach(p => p.classList.remove('active'));
	const pillMap = {
		'all': 'cat-pill-all',
		'Videos': 'cat-pill-videos',
		'Shorts': 'cat-pill-shorts',
		'Live Streams': 'cat-pill-live'
	};
	const activePill = document.getElementById(pillMap[category]);
	if (activePill) activePill.classList.add('active');
	renderStudioCatalog();
}

function setStudioStatusFilter(status) {
	studioStatusFilter = status;
	studioCurrentPage = 1;
	document.querySelectorAll('#studio-status-filters .filter-pill').forEach(p => p.classList.remove('active'));
	const pillMap = {
		'all': 'stat-pill-all',
		'unarchived': 'stat-pill-unarchived',
		'archived': 'stat-pill-archived'
	};
	const activePill = document.getElementById(pillMap[status]);
	if (activePill) activePill.classList.add('active');
	renderStudioCatalog();
}

function filterStudioCatalog() {
	studioCurrentPage = 1;
	renderStudioCatalog();
}

function goToStudioPage(page) {
	if (page < 1) page = 1;
	if (page > studioTotalPages) page = studioTotalPages;
	studioCurrentPage = page;
	renderStudioCatalog();
	const container = document.getElementById('studio-catalog-container');
	if (container) container.scrollTop = 0;
}

function renderStudioCatalog() {
	const container = document.getElementById('studio-catalog-container');
	if (!container) return;

	const searchInput = document.getElementById('studio-search-input');
	const query = (searchInput ? searchInput.value : '').toLowerCase().trim();

	const filtered = currentStudioCatalog.filter(v => {
		// Search filter
		if (query && !v.title.toLowerCase().includes(query)) {
			return false;
		}
		// Category filter
		if (studioCategoryFilter !== 'all') {
			if (v.category !== studioCategoryFilter) return false;
		}
		// Status filter
		if (studioStatusFilter === 'unarchived' && v.is_archived) return false;
		if (studioStatusFilter === 'archived' && !v.is_archived) return false;

		return true;
	});

	const paginationBar = document.getElementById('studio-pagination-bar');

	if (filtered.length === 0) {
		container.innerHTML = `
			<div class="studio-empty">
				<p>No videos matching current filters</p>
			</div>
		`;
		if (paginationBar) paginationBar.style.display = 'none';
		updateStudioBatchBar([]);
		return;
	}

	// Calculate pagination
	studioTotalPages = Math.max(1, Math.ceil(filtered.length / studioPageSize));
	if (studioCurrentPage > studioTotalPages) studioCurrentPage = studioTotalPages;
	if (studioCurrentPage < 1) studioCurrentPage = 1;

	const startIdx = (studioCurrentPage - 1) * studioPageSize;
	const pageItems = filtered.slice(startIdx, startIdx + studioPageSize);

	// Update pagination bar
	if (paginationBar) {
		if (filtered.length > studioPageSize) {
			paginationBar.style.display = 'flex';
			const rangeEl = document.getElementById('studio-page-range');
			const curEl = document.getElementById('studio-current-page-num');
			const totEl = document.getElementById('studio-total-pages-num');
			const btnFirst = document.getElementById('studio-btn-first');
			const btnPrev = document.getElementById('studio-btn-prev');
			const btnNext = document.getElementById('studio-btn-next');
			const btnLast = document.getElementById('studio-btn-last');

			if (rangeEl) rangeEl.textContent = `Showing ${startIdx + 1}–${Math.min(startIdx + pageItems.length, filtered.length)} of ${filtered.length}`;
			if (curEl) curEl.textContent = studioCurrentPage;
			if (totEl) totEl.textContent = studioTotalPages;
			if (btnFirst) btnFirst.disabled = (studioCurrentPage === 1);
			if (btnPrev) btnPrev.disabled = (studioCurrentPage === 1);
			if (btnNext) btnNext.disabled = (studioCurrentPage === studioTotalPages);
			if (btnLast) btnLast.disabled = (studioCurrentPage === studioTotalPages);
		} else {
			paginationBar.style.display = 'none';
		}
	}

	container.innerHTML = '';
	pageItems.forEach(v => {
		const row = document.createElement('div');
		const isSelected = studioSelectedVideos.has(v.id);
		row.className = `studio-video-row ${isSelected ? 'selected' : ''}`;
		row.id = `studio-video-row-${v.id}`;

		const durationStr = formatDuration(v.duration);
		let catClass = 'video';
		let catLabel = 'Video';
		if (v.category === 'Shorts') {
			catClass = 'shorts';
			catLabel = 'Shorts';
		} else if (v.category === 'Live Streams') {
			catClass = 'live';
			catLabel = 'Live';
		}

		let statusBadge = '';
		if (v.is_archived) {
			statusBadge = `<span class="studio-status-pill archived"><svg viewBox="0 0 24 24" width="12" height="12" fill="currentColor"><path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/></svg>Archived</span>`;
		} else if (v.archived_status === 'queued' || v.archived_status === 'downloading') {
			statusBadge = `<span class="studio-status-pill queued">Queued</span>`;
		} else {
			statusBadge = `<span class="studio-status-pill ready">Ready</span>`;
		}

		const isCheckboxDisabled = v.is_archived || v.archived_status === 'downloading';

		row.innerHTML = `
			<input type="checkbox" class="studio-video-checkbox" ${isSelected ? 'checked' : ''} ${isCheckboxDisabled ? 'disabled' : ''} onchange="toggleStudioVideoSelect('${v.id}', this.checked)" title="Select video">
			<div class="studio-thumb-wrapper">
				<img src="${escapeHTML(v.thumbnail || '')}" alt="${escapeHTML(v.title)}" loading="lazy"/>
				<span class="studio-cat-tag ${catClass}">${catLabel}</span>
				${durationStr ? `<span class="studio-duration-tag">${durationStr}</span>` : ''}
			</div>
			<div class="studio-video-info">
				<div class="studio-video-title" title="${escapeHTML(v.title)}">${escapeHTML(v.title)}</div>
				<div class="studio-video-meta">
					${statusBadge}
					<span>ID: ${escapeHTML(v.id)}</span>
				</div>
			</div>
			<div class="studio-video-actions">
				${!v.is_archived ? `
					<button class="btn btn-sm btn-secondary" onclick="enqueueSingleStudioVideo('${v.id}')" title="Download this video">
						<svg viewBox="0 0 24 24" width="13" height="13" fill="currentColor" style="vertical-align:text-top;margin-right:4px;"><path d="M19 9h-4V3H9v6H5l7 7 7-7zM5 18v2h14v-2H5z"/></svg>Download
					</button>
				` : `
					<button class="btn btn-sm btn-secondary" onclick="openDownloadsFolder()" title="Open download folder">
						<svg viewBox="0 0 24 24" width="13" height="13" fill="currentColor" style="vertical-align:text-top;margin-right:4px;"><path d="M10 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z"/></svg>Saved
					</button>
				`}
			</div>
		`;
		container.appendChild(row);
	});

	updateStudioBatchBar(filtered);
}

function getFilteredStudioCatalog() {
	const searchInput = document.getElementById('studio-search-input');
	const query = (searchInput ? searchInput.value : '').toLowerCase().trim();

	return currentStudioCatalog.filter(v => {
		if (query && !v.title.toLowerCase().includes(query)) return false;
		if (studioCategoryFilter !== 'all') {
			if (v.category !== studioCategoryFilter) return false;
		}
		if (studioStatusFilter === 'unarchived' && v.is_archived) return false;
		if (studioStatusFilter === 'archived' && !v.is_archived) return false;
		return true;
	});
}

function toggleStudioVideoSelect(videoId, isChecked) {
	if (isChecked) {
		studioSelectedVideos.add(videoId);
	} else {
		studioSelectedVideos.delete(videoId);
	}
	const row = document.getElementById(`studio-video-row-${videoId}`);
	if (row) {
		if (isChecked) row.classList.add('selected');
		else row.classList.remove('selected');
	}
	updateStudioBatchBar();
}

function toggleSelectAllStudioVideos(isChecked) {
	const filtered = getFilteredStudioCatalog();
	if (isChecked) {
		filtered.forEach(v => {
			if (!v.is_archived && v.archived_status !== 'downloading') {
				studioSelectedVideos.add(v.id);
			}
		});
	} else {
		studioSelectedVideos.clear();
	}
	renderStudioCatalog();
}

function selectAllChannelVideos(selectAll) {
	const filtered = getFilteredStudioCatalog();
	if (selectAll) {
		filtered.forEach(v => {
			if (!v.is_archived && v.archived_status !== 'downloading') {
				studioSelectedVideos.add(v.id);
			}
		});
	} else {
		studioSelectedVideos.clear();
	}
	renderStudioCatalog();
}

function selectCurrentPageVideos() {
	const filtered = getFilteredStudioCatalog();
	const startIdx = (studioCurrentPage - 1) * studioPageSize;
	const pageItems = filtered.slice(startIdx, startIdx + studioPageSize);
	pageItems.forEach(v => {
		if (!v.is_archived && v.archived_status !== 'downloading') {
			studioSelectedVideos.add(v.id);
		}
	});
	renderStudioCatalog();
}

function updateStudioBatchBar(filteredList) {
	const countEl = document.getElementById('studio-selected-count');
	const dlBtn = document.getElementById('studio-download-selected-btn');
	const dlLabel = document.getElementById('studio-download-selected-label');
	const selectAllCb = document.getElementById('studio-select-all');
	const totalUnarchivedEl = document.getElementById('studio-total-unarchived-count');
	const selectAllLabel = document.getElementById('studio-select-all-label');

	const list = filteredList || getFilteredStudioCatalog();
	const unarchived = list.filter(v => !v.is_archived && v.archived_status !== 'downloading');

	if (totalUnarchivedEl) {
		totalUnarchivedEl.textContent = unarchived.length;
	}

	const count = studioSelectedVideos.size;
	if (countEl) countEl.textContent = `${count} selected`;

	if (dlBtn && dlLabel) {
		dlLabel.textContent = `Download Selected (${count})`;
		dlBtn.disabled = (count === 0);
	}

	if (selectAllCb) {
		if (unarchived.length === 0) {
			selectAllCb.checked = false;
			selectAllCb.indeterminate = false;
		} else {
			const selectedInFilter = unarchived.filter(v => studioSelectedVideos.has(v.id));
			if (selectedInFilter.length === unarchived.length) {
				selectAllCb.checked = true;
				selectAllCb.indeterminate = false;
			} else if (selectedInFilter.length > 0) {
				selectAllCb.checked = false;
				selectAllCb.indeterminate = true;
			} else {
				selectAllCb.checked = false;
				selectAllCb.indeterminate = false;
			}
		}
	}

	if (selectAllLabel) {
		if (count > 0 && unarchived.length > 0 && count >= unarchived.length) {
			selectAllLabel.textContent = `All ${unarchived.length} Selected`;
		} else {
			selectAllLabel.textContent = 'Select All';
		}
	}
}

async function enqueueSelectedStudioVideos() {
	if (!currentStudioChannelId || studioSelectedVideos.size === 0) return;
	const videoIds = Array.from(studioSelectedVideos);

	showToast(`Queueing ${videoIds.length} selected videos…`, 'info');
	try {
		const res = await fetch(`/api/channels/${currentStudioChannelId}/enqueue-selected`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ video_ids: videoIds })
		});
		const data = await res.json();
		if (!res.ok || data.error) {
			throw new Error(data.error || 'Failed to enqueue videos');
		}

		showToast(data.message || `Queued ${videoIds.length} videos!`, 'success');
		studioSelectedVideos.clear();
		fetchDownloads();
		openChannelStudio(currentStudioChannelId);
	} catch (err) {
		showToast(err.message, 'error');
	}
}

async function enqueueSingleStudioVideo(videoId) {
	if (!currentStudioChannelId || !videoId) return;
	try {
		const res = await fetch(`/api/channels/${currentStudioChannelId}/enqueue-selected`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ video_ids: [videoId] })
		});
		const data = await res.json();
		if (!res.ok || data.error) {
			throw new Error(data.error || 'Failed to enqueue video');
		}

		showToast('Video added to download queue!', 'success');
		fetchDownloads();
		openChannelStudio(currentStudioChannelId);
	} catch (err) {
		showToast(err.message, 'error');
	}
}

async function syncLatestChannelVideos(count) {
	if (!currentStudioCatalog || currentStudioCatalog.length === 0) {
		showToast('No videos available in catalog', 'warning');
		return;
	}
	const unarchived = currentStudioCatalog.filter(v => !v.is_archived);
	if (unarchived.length === 0) {
		showToast('All available videos are already archived!', 'info');
		return;
	}
	const target = unarchived.slice(0, count).map(v => v.id);
	studioSelectedVideos = new Set(target);
	enqueueSelectedStudioVideos();
}

async function saveStudioRules() {
	if (!currentStudioChannelId) return;

	const autoDl = document.getElementById('rule-auto-download')?.checked || false;
	const exShorts = document.getElementById('rule-exclude-shorts')?.checked || false;
	const exLive = document.getElementById('rule-exclude-live')?.checked || false;
	const minDur = parseInt(document.getElementById('rule-min-duration')?.value || '0', 10);
	const maxSync = parseInt(document.getElementById('rule-max-auto-sync')?.value || '0', 10);

	try {
		const res = await fetch(`/api/channels/${currentStudioChannelId}/rules`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({
				auto_download: autoDl,
				min_duration_sec: minDur,
				exclude_shorts: exShorts,
				exclude_livestreams: exLive,
				max_auto_sync_count: maxSync
			})
		});
		const data = await res.json();
		if (!res.ok || data.error) {
			throw new Error(data.error || 'Failed to save rules');
		}

		showToast('Channel auto-archive rules saved!', 'success');
		fetchChannels();
	} catch (err) {
		showToast(err.message, 'error');
	}
}

// Cookie File Upload & Management
let isCookieFileLoaded = false;

async function uploadCookieFile(event) {
	const file = event.target.files[0];
	if (!file) return;

	const formData = new FormData();
	formData.append('cookies', file);

	try {
		const res = await fetch('/api/cookies/upload', {
			method: 'POST',
			body: formData
		});
		const data = await res.json();
		if (!res.ok || data.error) throw new Error(data.error || 'Upload failed');
		showToast('cookies.txt uploaded & activated!', 'success');
		
		isCookieFileLoaded = true;
		document.getElementById('cookie-file-status').textContent = `Active: ${file.name}`;
		document.getElementById('cookie-file-status').style.color = 'var(--accent-green, #22c55e)';
		const delBtn = document.getElementById('cookie-file-delete-btn');
		if (delBtn) delBtn.style.display = 'inline-block';
		document.getElementById('pref-cookie-browser').value = 'none';
	} catch (err) {
		showToast(err.message, 'error');
	}
}

async function deleteCookieFile() {
	try {
		const res = await fetch('/api/cookies', { method: 'DELETE' });
		if (!res.ok) throw new Error('Failed to delete cookie file');
		isCookieFileLoaded = false;
		document.getElementById('cookie-file-status').textContent = 'No custom cookies.txt loaded';
		document.getElementById('cookie-file-status').style.color = '';
		const delBtn = document.getElementById('cookie-file-delete-btn');
		if (delBtn) delBtn.style.display = 'none';
		showToast('Cookie file removed', 'info');
	} catch (err) {
		showToast(err.message, 'error');
	}
}

function onBrowserCookieChange(val) {
	if (val !== 'none' && isCookieFileLoaded) {
		showToast(`Browser profile set to ${val} (will take priority over cookies.txt)`, 'info');
	}
}

// Library View
function handleLibrarySearch(val) {
	librarySearchTerm = val.toLowerCase().trim();
	currentLibraryPage = 1;
	renderLibrary();
}

function renderLibrary() {
	if (!libraryGridContainer) return;
	libraryGridContainer.innerHTML = '';

	const completedItems = allDownloads.filter(d => d.status === 'completed');
	const filtered = completedItems.filter(d => {
		if (!librarySearchTerm) return true;
		return (d.title || '').toLowerCase().includes(librarySearchTerm) ||
			(d.channel || '').toLowerCase().includes(librarySearchTerm) ||
			(d.video_id || '').toLowerCase().includes(librarySearchTerm);
	});

	document.getElementById('library-stats-text').textContent = `${filtered.length} Archived Videos`;
	const paginationBar = document.getElementById('library-pagination-bar');

	if (filtered.length === 0) {
		libraryGridContainer.innerHTML = `
			<div class="empty-state" style="grid-column: 1 / -1;">
				<svg viewBox="0 0 24 24"><path d="M4 6H2v14c0 1.1.9 2 2 2h14v-2H4V6zm16-4H8c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zm-8 12.5v-9l6 4.5-6 4.5z"/></svg>
				<h3>No Archived Items Found</h3>
				<p>Downloaded videos with generated offline players will appear here.</p>
			</div>
		`;
		if (paginationBar) paginationBar.style.display = 'none';
		return;
	}

	const total = filtered.length;
	totalLibraryPages = Math.ceil(total / libraryPageSize) || 1;
	if (currentLibraryPage > totalLibraryPages) currentLibraryPage = totalLibraryPages;
	if (currentLibraryPage < 1) currentLibraryPage = 1;

	const startIdx = (currentLibraryPage - 1) * libraryPageSize;
	const endIdx = Math.min(startIdx + libraryPageSize, total);
	const pageItems = filtered.slice(startIdx, endIdx);

	pageItems.forEach(item => {
		const card = document.createElement('div');
		card.className = 'lib-card';
		card.innerHTML = `
			<div class="lib-thumb-box">
				<img src="${item.thumbnail_url || ''}" alt="Thumbnail" onerror="this.src='data:image/svg+xml;utf8,<svg xmlns=\\'http://www.w3.org/2000/svg\\' width=\\'300\\' height=\\'170\\' fill=\\'%23111\\'></svg>'"/>
				<span class="lib-duration-badge">${formatDuration(item.duration)}</span>
				<span class="lib-format-badge">${item.is_audio_only ? 'AUDIO' : (item.quality || '1080P')}</span>
			</div>
			<div class="lib-body">
				<div>
					<div class="lib-title" title="${escapeHTML(item.title)}">${escapeHTML(item.title)}</div>
					<div class="lib-channel">${escapeHTML(item.channel || 'YouTube')} &bull; ${item.comments_count || 0} Comments</div>
				</div>
				<div class="lib-footer-actions">
					<button class="btn btn-sm btn-primary" style="flex:1;" onclick="openHTMLPlayer('${item.id}')">
						<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor" style="vertical-align:text-top;margin-right:5px;"><path d="M8 5v14l11-7z"/></svg>Watch Offline
					</button>
					<button class="btn btn-sm btn-secondary" onclick="openFolder('${item.id}')" title="Open Directory">
						<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor"><path d="M10 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z"/></svg>
					</button>
					<button class="btn btn-sm btn-danger" onclick="deleteDownload('${item.id}', true)" title="Delete Files">
						<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor"><path d="M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/></svg>
					</button>
				</div>
			</div>
		`;
		libraryGridContainer.appendChild(card);
	});

	if (paginationBar) {
		if (total > libraryPageSize) {
			paginationBar.style.display = 'flex';
			document.getElementById('library-page-range').textContent = `Showing ${startIdx + 1}–${endIdx} of ${total} videos`;
			document.getElementById('lib-current-page-num').textContent = currentLibraryPage;
			document.getElementById('lib-total-pages-num').textContent = totalLibraryPages;
			document.getElementById('lib-btn-first').disabled = currentLibraryPage <= 1;
			document.getElementById('lib-btn-prev').disabled = currentLibraryPage <= 1;
			document.getElementById('lib-btn-next').disabled = currentLibraryPage >= totalLibraryPages;
			document.getElementById('lib-btn-last').disabled = currentLibraryPage >= totalLibraryPages;
		} else {
			paginationBar.style.display = 'none';
		}
	}
}

function goToLibraryPage(page) {
	if (page < 1 || page > totalLibraryPages) return;
	currentLibraryPage = page;
	renderLibrary();
	window.scrollTo({ top: 0, behavior: 'smooth' });
}

function switchPrefCategory(category) {
	const pills = document.querySelectorAll('.pref-subnav-pill');
	const panes = document.querySelectorAll('.pref-category-pane');

	pills.forEach(pill => {
		const isActive = pill.dataset.prefTarget === category;
		pill.classList.toggle('active', isActive);
		if (isActive) {
			pill.scrollIntoView({ behavior: 'smooth', inline: 'nearest', block: 'nearest' });
		}
	});

	panes.forEach(pane => {
		const isTarget = pane.dataset.prefCategory === category;
		pane.classList.toggle('active', isTarget);
		if (isTarget) {
			pane.style.animation = 'none';
			pane.offsetHeight;
			pane.style.animation = '';
		}
	});

	localStorage.setItem('yt_archiver_pref_tab', category);
}

function setAdvancedPreferences(visible) {
	advancedPreferencesVisible = visible;
	const container = document.querySelector('.preferences-container');
	if (container) container.classList.toggle('advanced-preferences-hidden', !visible);
	const btn = document.getElementById('advanced-toggle-btn');
	if (btn) btn.textContent = visible ? 'Hide advanced settings' : 'Show advanced settings';
	localStorage.setItem('yt_archiver_advanced_preferences', visible ? 'true' : 'false');
}

function toggleAdvancedPreferences() { setAdvancedPreferences(!advancedPreferencesVisible); }

function queuePreferenceSave() {
	if (!preferencesLoaded) return;
	clearTimeout(preferencesSaveTimer);
	preferencesSaveTimer = setTimeout(() => savePreferences(null, true), 800);
}

function updatePreferencesOverview(prefs) {
	const grid = document.getElementById('pref-overview-grid'); if (!grid) return;
	const profileName = document.getElementById('quick-profile-select')?.selectedOptions[0]?.textContent || 'Custom defaults';
	const cards = [
		[`<svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor"><path d="M18 4l2 4h-3l-2-4h-2l2 4h-3l-2-4H8l2 4H7L5 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V4h-4z"/></svg>`, 'Download defaults', `${prefs.download_audio_only ? 'Audio' : prefs.video_quality || '1080p'} • ${prefs.download_audio_only ? prefs.audio_format || 'MP3' : prefs.video_format || 'MP4'}`, 'media', 'Edit defaults'],
		[`<svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor"><path d="M12 1L3 5v6c0 5.55 3.84 10.74 9 12 5.16-1.26 9-6.45 9-12V5l-9-4zm-2 16l-4-4 1.41-1.41L10 14.17l6.59-6.59L18 9l-8 8z"/></svg>`, 'Automation', prefs.auto_sync_channels ? `Channel sync every ${prefs.sync_interval_minutes || 60} min` : 'Channel sync is off', 'automation', prefs.auto_sync_channels ? 'Review schedule' : 'Set up automation'],
		[`<svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10c0-.34-.02-.67-.06-1-.53.19-1.1.3-1.69.3-2.76 0-5-2.24-5-5 0-1.32.51-2.52 1.34-3.41C15.93 2.37 14.04 2 12 2zm-3.5 6c.83 0 1.5.67 1.5 1.5S9.33 11 8.5 11 7 10.33 7 9.5 7.67 8 8.5 8zm-1 7c-.83 0-1.5-.67-1.5-1.5S6.67 12 7.5 12s1.5.67 1.5 1.5S8.33 15 7.5 15zm5.5 2c-.83 0-1.5-.67-1.5-1.5s.67-1.5 1.5-1.5 1.5.67 1.5 1.5-.67 1.5-1.5 1.5zm2-6c-.83 0-1.5-.67-1.5-1.5S14.17 8 15 8s1.5.67 1.5 1.5-.67 1.5-1.5 1.5z"/></svg>`, 'Privacy & sign-in', prefs.cookie_source === 'file' ? 'cookies.txt connected' : prefs.cookie_source === 'browser' ? 'Browser session connected' : 'No account credentials connected', 'cookies', 'Manage privacy'],
		[`<svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor"><path d="M19.35 10.04C18.67 6.59 15.64 4 12 4 9.11 4 6.6 5.64 5.35 8.04 2.34 8.36 0 10.91 0 14c0 3.31 2.69 6 6 6h13c2.76 0 5-2.24 5-5 0-2.64-2.05-4.78-4.65-4.96zM19 18H6c-2.21 0-4-1.79-4-4 0-2.05 1.53-3.76 3.56-3.97l1.07-.11.5-.95C8.08 7.14 9.94 6 12 6c2.62 0 4.88 1.86 5.39 4.43l.3 1.5 1.53.11c1.56.1 2.78 1.41 2.78 2.96 0 1.65-1.35 3-3 3z"/></svg>`, 'Storage & profile', `${profileName} • ${prefs.download_folder || 'Choose a folder'}`, 'storage', 'Manage storage']
	];
	grid.innerHTML = cards.map(c => `<div class="overview-card"><div class="overview-card-top"><span class="overview-icon">${c[0]}</span><div><h4>${c[1]}</h4><p>${escapeHTML(c[2])}</p></div></div><button type="button" class="btn btn-xs btn-secondary" onclick="switchPrefCategory('${c[3]}')">${c[4]}</button></div>`).join('');
}

// Preferences
async function loadPreferences() {
	try {
		const savedTab = localStorage.getItem('yt_archiver_pref_tab') || 'overview';
		switchPrefCategory(savedTab);

		const res = await fetch('/api/preferences');
		if (!res.ok) return;
		const prefs = await res.json();

		document.getElementById('pref-video-format').value = prefs.video_format || 'mp4';
		document.getElementById('pref-video-quality').value = prefs.video_quality || '1080p';
		document.getElementById('pref-audio-format').value = prefs.audio_format || 'mp3';
		document.getElementById('pref-audio-quality').value = prefs.audio_quality || '320k';

		document.getElementById('pref-cookie-browser').value = prefs.cookie_browser || 'none';

		// Show cookie file status if a file is loaded
		const cookieStatus = document.getElementById('cookie-file-status');
		const delBtn = document.getElementById('cookie-file-delete-btn');
		if (prefs.cookie_file_path) {
			isCookieFileLoaded = true;
			if (cookieStatus) {
				cookieStatus.textContent = 'Active: cookies.txt';
				cookieStatus.style.color = 'var(--accent-green, #22c55e)';
			}
			if (delBtn) delBtn.style.display = 'inline-block';
			if (prefs.cookie_source === 'file') {
				document.getElementById('pref-cookie-browser').value = 'none';
			}
		} else {
			isCookieFileLoaded = false;
			if (cookieStatus) {
				cookieStatus.textContent = 'No custom cookies.txt loaded';
				cookieStatus.style.color = '';
			}
			if (delBtn) delBtn.style.display = 'none';
		}
		document.getElementById('pref-sponsorblock-action').value = prefs.sponsorblock_action || 'mark';
		document.getElementById('pref-sponsorblock-categories').value = prefs.sponsorblock_categories || 'sponsor,selfpromo,intro,outro';

		document.getElementById('pref-embed-metadata').checked = prefs.embed_metadata !== false;
		document.getElementById('pref-embed-cover-art').checked = prefs.embed_cover_art !== false;
		document.getElementById('pref-embed-chapters').checked = prefs.embed_chapters !== false;
		document.getElementById('pref-embed-subtitles').checked = prefs.embed_subtitles === true;

		if (document.getElementById('pref-extract-companion-audio')) {
			document.getElementById('pref-extract-companion-audio').checked = prefs.extract_companion_audio !== false;
		}
		if (document.getElementById('pref-companion-audio-format')) {
			document.getElementById('pref-companion-audio-format').value = prefs.companion_audio_format || 'mp3';
		}
		if (document.getElementById('pref-generate-nfo')) {
			document.getElementById('pref-generate-nfo').checked = prefs.generate_nfo !== false;
		}

		document.getElementById('pref-download-thumbnail').checked = prefs.download_thumbnail !== false;
		document.getElementById('pref-download-metadata').checked = prefs.download_metadata !== false;
		document.getElementById('pref-download-subtitles').checked = prefs.download_subtitles !== false;
		document.getElementById('pref-subtitle-langs').value = prefs.subtitle_langs || 'en,auto';
		if (document.getElementById('pref-fetch-dislikes')) {
			document.getElementById('pref-fetch-dislikes').checked = prefs.fetch_dislikes !== false;
		}

		document.getElementById('pref-download-comments').checked = prefs.download_comments !== false;
		const knownCommentLimits = ['100', '250', '500', '1000', '2500', '5000', '10000', '25000', '50000', '-1'];
		const commLimitVal = (prefs.comment_limit !== undefined ? prefs.comment_limit : 1000).toString();
		const prefCommSelect = document.getElementById('pref-comment-limit');
		const prefCommCustom = document.getElementById('pref-comment-limit-custom');
		if (prefCommSelect) {
			if (knownCommentLimits.includes(commLimitVal)) {
				prefCommSelect.value = commLimitVal;
				if (prefCommCustom) {
					prefCommCustom.style.display = 'none';
					prefCommCustom.value = commLimitVal === '-1' ? '' : commLimitVal;
				}
			} else {
				prefCommSelect.value = 'custom';
				if (prefCommCustom) {
					prefCommCustom.style.display = 'block';
					prefCommCustom.value = commLimitVal;
				}
			}
		}
		document.getElementById('pref-download-avatars').checked = prefs.download_commenter_avatars !== false;

		document.getElementById('pref-generate-html').checked = prefs.generate_html_report !== false;
		document.getElementById('pref-download-folder').value = prefs.download_folder || '';
		document.getElementById('pref-duplicate-action').value = prefs.duplicate_action || 'skip';
		document.getElementById('pref-max-concurrent').value = prefs.max_concurrent_downloads || 2;
		document.getElementById('pref-speed-limit').value = prefs.speed_limit || '';

		// Auto-Retry Failed Downloads
		document.getElementById('pref-auto-retry-failed').checked = prefs.auto_retry_failed === true;
		document.getElementById('pref-auto-retry-interval').value = prefs.auto_retry_interval_minutes || 15;
		document.getElementById('pref-auto-retry-max-attempts').value = prefs.auto_retry_max_attempts || 3;
		if (document.getElementById('pref-circuit-breaker-enabled')) {
			document.getElementById('pref-circuit-breaker-enabled').checked = prefs.circuit_breaker_enabled !== false;
		}
		document.getElementById('pref-auto-sync').checked = prefs.auto_sync_channels === true;
		document.getElementById('pref-sync-interval').value = prefs.sync_interval_minutes || 60;
		document.getElementById('pref-sync-window-enabled').checked = prefs.sync_window_enabled === true;
		document.getElementById('pref-sync-window-start').value = prefs.sync_window_start || '01:00';
		document.getElementById('pref-sync-window-end').value = prefs.sync_window_end || '06:00';
		document.getElementById('pref-download-window-enabled').checked = prefs.download_window_enabled === true;
		document.getElementById('pref-download-window-start').value = prefs.download_window_start || '01:00';
		document.getElementById('pref-download-window-end').value = prefs.download_window_end || '06:00';

		// UI Density Mode
		applyUIMode(prefs.ui_mode || 'standard');
		applyTheme(prefs.theme || localStorage.getItem('yt_archiver_theme') || 'midnight', prefs.color_scheme || localStorage.getItem('yt_archiver_color_scheme') || 'crimson');

		// Also pre-populate the Quick Options row on the New Download tab with your global defaults
		if (document.getElementById('quick-type-select')) {
			document.getElementById('quick-type-select').value = prefs.download_audio_only ? 'audio' : 'video';
			toggleQuickType(prefs.download_audio_only ? 'audio' : 'video');
		}
		if (document.getElementById('quick-quality-select')) {
			document.getElementById('quick-quality-select').value = prefs.video_quality || '1080p';
		}
		if (document.getElementById('quick-audio-fmt-select')) {
			document.getElementById('quick-audio-fmt-select').value = prefs.audio_format || 'mp3';
		}
		const quickCommSelect = document.getElementById('quick-comments-select');
		const quickCommCustom = document.getElementById('quick-comments-custom');
		if (quickCommSelect) {
			if (prefs.download_comments === false) {
				quickCommSelect.value = '0';
				if (quickCommCustom) quickCommCustom.style.display = 'none';
			} else if (knownCommentLimits.includes(commLimitVal)) {
				quickCommSelect.value = commLimitVal;
				if (quickCommCustom) quickCommCustom.style.display = 'none';
			} else {
				quickCommSelect.value = 'custom';
				if (quickCommCustom) {
					quickCommCustom.style.display = 'inline-block';
					quickCommCustom.value = commLimitVal;
				}
			}
		}
		if (document.getElementById('quick-html-toggle')) {
			document.getElementById('quick-html-toggle').checked = prefs.generate_html_report !== false;
		}
		dismissedFeatureCards = prefs.dismissed_feature_cards || [];
		applyDismissedFeatureCards();
		syncCustomSelects();
		preferencesLoaded = true;
		updatePreferencesOverview(prefs);
	} catch (err) {
		console.error('Failed to load preferences', err);
	}
}

function applyDismissedFeatureCards() {
	const allCardIds = ['feature-card-offline', 'feature-card-sync', 'feature-card-queue'];
	let visibleCount = 0;
	allCardIds.forEach(id => {
		const card = document.getElementById(id);
		if (!card) return;
		if (dismissedFeatureCards.includes(id)) {
			card.style.display = 'none';
		} else {
			card.style.display = '';
			visibleCount++;
		}
	});
	const grid = document.getElementById('features-grid');
	if (grid) {
		grid.style.display = visibleCount > 0 ? 'grid' : 'none';
	}
}

function dismissFeatureCard(cardId) {
	const card = document.getElementById(cardId);
	if (card) {
		card.classList.add('dismissing');
		setTimeout(() => {
			card.style.display = 'none';
			const grid = document.getElementById('features-grid');
			const remaining = grid ? grid.querySelectorAll('.feature-card:not([style*="display: none"])').length : 0;
			if (grid && remaining === 0) grid.style.display = 'none';
		}, 250);
	}
	if (!dismissedFeatureCards.includes(cardId)) {
		dismissedFeatureCards.push(cardId);
	}
	savePreferences(null, true);
}

function handlePrefCommentLimitChange(val) {
	const customInput = document.getElementById('pref-comment-limit-custom');
	if (!customInput) return;
	if (val === 'custom') {
		customInput.style.display = 'block';
		if (!customInput.value) customInput.value = '10000';
		customInput.focus();
	} else {
		customInput.style.display = 'none';
	}
	queuePreferenceSave();
}

function handleQuickCommentsChange(val) {
	const customInput = document.getElementById('quick-comments-custom');
	if (!customInput) return;
	if (val === 'custom') {
		customInput.style.display = 'inline-block';
		if (!customInput.value) customInput.value = '10000';
		customInput.focus();
	} else {
		customInput.style.display = 'none';
	}
}

async function savePreferences(e, silent = false) {
	if (e) e.preventDefault();

	const browserVal = document.getElementById('pref-cookie-browser').value;
	let cookieSource = 'none';
	if (browserVal !== 'none') {
		cookieSource = 'browser';
	} else if (isCookieFileLoaded) {
		cookieSource = 'file';
	}

	const speedInput = document.getElementById('pref-speed-limit');
	const speed = speedInput.value.trim();
	if (speed && !/^\d+(\.\d+)?\s*[kmg]?$/i.test(speed)) {
		speedInput.classList.add('invalid');
		showToast('Use a valid speed limit (for example 5M, 500K).', 'error');
		return;
	}
	speedInput.classList.remove('invalid');

	let commentLimit = 1000;
	const prefCommVal = document.getElementById('pref-comment-limit') ? document.getElementById('pref-comment-limit').value : '1000';
	if (prefCommVal === 'custom') {
		commentLimit = parseInt(document.getElementById('pref-comment-limit-custom')?.value, 10) || 1000;
	} else {
		commentLimit = parseInt(prefCommVal, 10);
	}

	const prefs = {
		download_video: true,
		video_format: document.getElementById('pref-video-format').value,
		video_quality: document.getElementById('pref-video-quality').value,
		download_audio_only: false,
		audio_format: document.getElementById('pref-audio-format').value,
		audio_quality: document.getElementById('pref-audio-quality').value,

		cookie_source: cookieSource,
		cookie_browser: browserVal,
		sponsorblock_action: document.getElementById('pref-sponsorblock-action').value,
		sponsorblock_categories: document.getElementById('pref-sponsorblock-categories').value,

		embed_metadata: document.getElementById('pref-embed-metadata').checked,
		embed_cover_art: document.getElementById('pref-embed-cover-art').checked,
		embed_chapters: document.getElementById('pref-embed-chapters').checked,
		embed_subtitles: document.getElementById('pref-embed-subtitles').checked,
		extract_companion_audio: document.getElementById('pref-extract-companion-audio') ? document.getElementById('pref-extract-companion-audio').checked : true,
		companion_audio_format: document.getElementById('pref-companion-audio-format') ? document.getElementById('pref-companion-audio-format').value : 'mp3',
		generate_nfo: document.getElementById('pref-generate-nfo') ? document.getElementById('pref-generate-nfo').checked : true,

		download_thumbnail: document.getElementById('pref-download-thumbnail').checked,
		download_metadata: document.getElementById('pref-download-metadata').checked,
		download_subtitles: document.getElementById('pref-download-subtitles').checked,
		subtitle_langs: document.getElementById('pref-subtitle-langs').value,
		fetch_dislikes: document.getElementById('pref-fetch-dislikes') ? document.getElementById('pref-fetch-dislikes').checked : true,

		download_comments: document.getElementById('pref-download-comments').checked,
		comment_limit: commentLimit,
		download_commenter_avatars: document.getElementById('pref-download-avatars').checked,

		generate_html_report: document.getElementById('pref-generate-html').checked,
		download_folder: document.getElementById('pref-download-folder').value,
		duplicate_action: document.getElementById('pref-duplicate-action').value,
		max_concurrent_downloads: parseInt(document.getElementById('pref-max-concurrent').value, 10),
		speed_limit: speed.replace(/\s/g, '').toUpperCase(),

		auto_retry_failed: document.getElementById('pref-auto-retry-failed').checked,
		auto_retry_interval_minutes: parseInt(document.getElementById('pref-auto-retry-interval').value, 10),
		auto_retry_max_attempts: parseInt(document.getElementById('pref-auto-retry-max-attempts').value, 10),
		circuit_breaker_enabled: document.getElementById('pref-circuit-breaker-enabled') ? document.getElementById('pref-circuit-breaker-enabled').checked : true,
		auto_sync_channels: document.getElementById('pref-auto-sync').checked,
		sync_interval_minutes: parseInt(document.getElementById('pref-sync-interval').value, 10),
		sync_window_enabled: document.getElementById('pref-sync-window-enabled').checked,
		sync_window_start: document.getElementById('pref-sync-window-start').value,
		sync_window_end: document.getElementById('pref-sync-window-end').value,
		download_window_enabled: document.getElementById('pref-download-window-enabled').checked,
		download_window_start: document.getElementById('pref-download-window-start').value,
		download_window_end: document.getElementById('pref-download-window-end').value,
		ui_mode: document.getElementById('pref-ui-mode').value
		,theme: document.getElementById('pref-theme').value
		,color_scheme: document.getElementById('pref-color-scheme').value
		,dismissed_feature_cards: dismissedFeatureCards
	};

	try {
		const res = await fetch('/api/preferences', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(prefs)
		});

		if (!res.ok) throw new Error('Failed to save preferences');
		updatePreferencesOverview(prefs);
		if (!silent) showToast('Preferences saved', 'success');
	} catch (err) {
		showToast('Failed to save settings: ' + err.message, 'error');
	}
}

async function loadProfiles() {
	try {
		const res = await fetch('/api/profiles'); if (!res.ok) return;
		downloadProfiles = await res.json();
		const select = document.getElementById('quick-profile-select'); if (!select) return;
		select.innerHTML = '<option value="">Custom / Defaults</option>' + downloadProfiles.map(p => `<option value="${escapeHTML(p.id)}">${escapeHTML(p.name)}</option>`).join('');
		refreshCustomSelect(select);
	} catch (err) { console.error('Failed to load profiles', err); }
}

function applyQuickProfile(id) {
	const profile = downloadProfiles.find(p => p.id === id); if (!profile) return;
	const p = profile.preferences || {};
	document.getElementById('quick-type-select').value = p.download_audio_only ? 'audio' : 'video';
	toggleQuickType(p.download_audio_only ? 'audio' : 'video');
	if (p.video_quality) document.getElementById('quick-quality-select').value = p.video_quality;
	if (p.audio_format) document.getElementById('quick-audio-fmt-select').value = p.audio_format;
	if (p.comment_limit !== undefined) document.getElementById('quick-comments-select').value = p.comment_limit;
	if (p.generate_html_report !== undefined) document.getElementById('quick-html-toggle').checked = p.generate_html_report;
	showToast(`${profile.name} applied`, 'success');
}

async function saveCurrentAsProfile() {
	const name = await showPromptDialog({
		title: 'Save Quick Profile',
		message: 'Create a reusable preset containing your current video, audio, comment, and format preferences.',
		placeholder: 'e.g. 1080p Archive, Audio Only, Ultra 4K',
		confirmText: 'Save Profile',
		cancelText: 'Cancel'
	});
	if (!name || !name.trim()) return;

	const prefs = await (await fetch('/api/preferences')).json();
	Object.assign(prefs, {
		video_format: document.getElementById('pref-video-format').value,
		video_quality: document.getElementById('pref-video-quality').value,
		audio_format: document.getElementById('pref-audio-format').value,
		audio_quality: document.getElementById('pref-audio-quality').value
	});
	downloadProfiles.push({ id: `profile-${Date.now()}`, name: name.trim(), preferences: prefs });
	const res = await fetch('/api/profiles', {
		method: 'POST',
		headers: {'Content-Type':'application/json'},
		body: JSON.stringify(downloadProfiles)
	});
	if (!res.ok) { showToast('Could not save profile', 'error'); return; }
	loadProfiles();
	showToast(`Profile "${name.trim()}" saved!`, 'success');
}

async function manageProfiles() {
	if (!downloadProfiles.length) {
		showToast('No profiles yet. Save your current settings to create one.', 'info');
		return;
	}

	const backdrop = document.getElementById('custom-dialog-backdrop');
	const iconEl = document.getElementById('custom-dialog-icon');
	const titleEl = document.getElementById('custom-dialog-title');
	const msgEl = document.getElementById('custom-dialog-message');
	const bodyEl = document.getElementById('custom-dialog-body');
	const actionsEl = document.getElementById('custom-dialog-actions');

	iconEl.className = 'custom-dialog-icon';
	iconEl.innerHTML = `<svg viewBox="0 0 24 24"><path d="M12 4V1L8 5l4 4V6c3.31 0 6 2.69 6 6 0 1.01-.25 1.97-.7 2.8l1.46 1.46C19.54 15.03 20 13.57 20 12c0-4.42-3.58-8-8-8zm0 14c-3.31 0-6-2.69-6-6 0-1.01.25-1.97.7-2.8L5.24 7.74C4.46 8.97 4 10.43 4 12c0 4.42 3.58 8 8 8v3l4-4-4-4v3z"/></svg>`;
	titleEl.textContent = 'Manage Quick Profiles';
	msgEl.textContent = 'View, apply, or remove your saved download presets.';

	bodyEl.innerHTML = `
		<div class="dialog-profiles-list">
			${downloadProfiles.map((p, i) => {
				const isAudio = p.preferences?.download_audio_only;
				const summary = isAudio ? `Audio Only (${p.preferences?.audio_format || 'MP3'})` : `${p.preferences?.video_quality || 'Best'} • ${p.preferences?.video_format || 'MP4'}`;
				return `
					<div class="dialog-profile-item">
						<div class="dialog-profile-item-info">
							<strong>${escapeHTML(p.name)}</strong>
							<span>${escapeHTML(summary)}</span>
						</div>
						<div class="dialog-profile-item-actions">
							<button type="button" class="btn btn-sm btn-secondary" onclick="applyProfileFromDialog('${p.id}')">Apply</button>
							<button type="button" class="btn btn-sm btn-danger" onclick="deleteProfileFromDialog(${i})" title="Delete Profile">
								<svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor"><path d="M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/></svg>
							</button>
						</div>
					</div>
				`;
			}).join('')}
		</div>
	`;

	actionsEl.innerHTML = `
		<button type="button" class="btn btn-secondary" onclick="closeCustomDialog(null)">Close</button>
		<button type="button" class="btn btn-primary" onclick="closeCustomDialog(null); saveCurrentAsProfile();">Save Current as New</button>
	`;

	backdrop.style.display = 'flex';
}

function applyProfileFromDialog(profileId) {
	applyQuickProfile(profileId);
	closeCustomDialog(null);
}

async function deleteProfileFromDialog(index) {
	if (index < 0 || index >= downloadProfiles.length) return;
	const name = downloadProfiles[index].name;
	downloadProfiles.splice(index, 1);
	await fetch('/api/profiles', {
		method: 'POST',
		headers: {'Content-Type':'application/json'},
		body: JSON.stringify(downloadProfiles)
	});
	loadProfiles();
	showToast(`Deleted profile "${name}"`, 'info');
	if (downloadProfiles.length === 0) {
		closeCustomDialog(null);
	} else {
		manageProfiles();
	}
}

async function checkEngineAndOpen() {
	switchPrefCategory('storage');
	setTimeout(() => {
		const targetCard = document.getElementById('engine-health-status')?.closest('.pref-card');
		if (targetCard) targetCard.scrollIntoView({ behavior: 'smooth', block: 'center' });
	}, 120);
	checkEngineHealth();
}

async function checkEngineHealth() {
	const el = document.getElementById('engine-health-status');
	if (el) el.textContent = 'Checking engine…';
	showToast('Checking engine components…', 'info');
	try {
		const r = await fetch('/api/engine/health');
		if (!r.ok) throw new Error(`HTTP ${r.status}`);
		const d = await r.json();
		const statusLine = `yt-dlp: ${d.yt_dlp} • FFmpeg: ${d.ffmpeg} • JS Solver: ${d.js_runtime}`;
		if (el) el.textContent = statusLine;
		const logStatus = document.getElementById('log-file-status');
		if (logStatus && d.log_file) {
			logStatus.textContent = `Active Session: ${d.log_file}`;
		}
		const ytdlpShort = (d.yt_dlp || '').split('\n')[0].replace(/^yt-dlp\s*/i, '');
		const ffmpegShort = (d.ffmpeg || '').split('\n')[0].replace(/^ffmpeg version\s*/i, 'v');
		showToast(`Engine Healthy: yt-dlp ${ytdlpShort}, FFmpeg ${ffmpegShort}`, 'success');
	} catch (err) {
		if (el) el.textContent = 'Health check failed';
		showToast('Engine health check failed: ' + err.message, 'error');
	}
}

async function viewLiveLog() {
	window.open('/api/logs/latest', '_blank');
}

async function openLogsFolder() {
	try {
		const r = await fetch('/api/logs/open-folder', { method: 'POST' });
		if (r.ok) {
			showToast('Opened logs folder in File Explorer', 'success');
		} else {
			showToast('Could not open folder automatically', 'info');
		}
	} catch (e) {
		showToast('Failed to open logs folder: ' + e.message, 'error');
	}
}
async function updateYtDlp() { showToast('Updating yt-dlp…', 'info'); try { const r = await fetch('/api/engine/update', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({engine:'yt-dlp'})}); const d = await r.json(); if (!r.ok) throw new Error(d.error); showToast('yt-dlp update complete', 'success'); checkEngineHealth(); } catch (e) { showToast(e.message, 'error'); } }
async function importBackup(event) { const file = event.target.files[0]; if (!file) return; try { const r = await fetch('/api/data/import', {method:'POST', headers:{'Content-Type':'application/json'}, body:await file.text()}); const d = await r.json(); if (!r.ok) throw new Error(d.error); showToast(d.message, 'success'); loadPreferences(); loadProfiles(); fetchChannels(); fetchDownloads(); } catch(e) { showToast(e.message, 'error'); } finally { event.target.value = ''; } }

async function resetPreferencesToDefault() {
	const confirmed = await showConfirmDialog({
		title: 'Reset to Factory Defaults?',
		message: 'This will restore all video formats, audio bitrates, privacy settings, and automation rules to default recommendations.',
		confirmText: 'Reset to Defaults',
		cancelText: 'Cancel',
		danger: true
	});
	if (!confirmed) return;

	try {
		const res = await fetch('/api/preferences/reset', { method: 'POST' });
		if (!res.ok) throw new Error('Failed to reset preferences');
		showToast('Preferences restored to factory defaults', 'success');
		loadPreferences();
	} catch (err) {
		showToast(err.message, 'error');
	}
}

// Custom Theme-Appropriate Modal & Dialog System
let customDialogResolver = null;

function closeCustomDialog(result) {
	const backdrop = document.getElementById('custom-dialog-backdrop');
	if (backdrop) backdrop.style.display = 'none';
	if (customDialogResolver) {
		const r = customDialogResolver;
		customDialogResolver = null;
		r(result);
	}
}

function handleDialogBackdropClick(e) {
	if (e.target.id === 'custom-dialog-backdrop') {
		closeCustomDialog(null);
	}
}

function showConfirmDialog({ title, message, icon, confirmText = 'Confirm', cancelText = 'Cancel', danger = false }) {
	return new Promise((resolve) => {
		customDialogResolver = resolve;
		const backdrop = document.getElementById('custom-dialog-backdrop');
		const iconEl = document.getElementById('custom-dialog-icon');
		const titleEl = document.getElementById('custom-dialog-title');
		const msgEl = document.getElementById('custom-dialog-message');
		const bodyEl = document.getElementById('custom-dialog-body');
		const actionsEl = document.getElementById('custom-dialog-actions');

		iconEl.className = danger ? 'custom-dialog-icon danger' : 'custom-dialog-icon';
		iconEl.innerHTML = icon || (danger
			? `<svg viewBox="0 0 24 24"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-2h2v2zm0-4h-2V7h2v6z"/></svg>`
			: `<svg viewBox="0 0 24 24"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15l-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z"/></svg>`);

		titleEl.textContent = title || 'Confirm Action';
		msgEl.textContent = message || 'Are you sure you want to proceed?';
		bodyEl.innerHTML = '';

		const confirmBtnClass = danger ? 'btn btn-danger' : 'btn btn-primary';
		actionsEl.innerHTML = `
			<button type="button" class="btn btn-secondary" onclick="closeCustomDialog(false)">${escapeHTML(cancelText)}</button>
			<button type="button" class="${confirmBtnClass}" id="custom-dialog-confirm-btn" onclick="closeCustomDialog(true)">${escapeHTML(confirmText)}</button>
		`;

		backdrop.style.display = 'flex';
		setTimeout(() => {
			const btn = document.getElementById('custom-dialog-confirm-btn');
			if (btn) btn.focus();
		}, 50);
	});
}

function showPromptDialog({ title, message, placeholder = '', defaultValue = '', confirmText = 'Save', cancelText = 'Cancel' }) {
	return new Promise((resolve) => {
		customDialogResolver = resolve;
		const backdrop = document.getElementById('custom-dialog-backdrop');
		const iconEl = document.getElementById('custom-dialog-icon');
		const titleEl = document.getElementById('custom-dialog-title');
		const msgEl = document.getElementById('custom-dialog-message');
		const bodyEl = document.getElementById('custom-dialog-body');
		const actionsEl = document.getElementById('custom-dialog-actions');

		iconEl.className = 'custom-dialog-icon';
		iconEl.innerHTML = `<svg viewBox="0 0 24 24"><path d="M3 17.25V21h3.75L17.81 9.94l-3.75-3.75L3 17.25zM20.71 7.04c.39-.39.39-1.02 0-1.41l-2.34-2.34c-.39-.39-1.02-.39-1.41 0l-1.83 1.83 3.75 3.75 1.83-1.83z"/></svg>`;

		titleEl.textContent = title || 'Enter Input';
		msgEl.textContent = message || '';
		bodyEl.innerHTML = `
			<input type="text" id="custom-dialog-prompt-input" class="dialog-input" placeholder="${escapeHTML(placeholder)}" value="${escapeHTML(defaultValue)}" autocomplete="off" />
		`;

		actionsEl.innerHTML = `
			<button type="button" class="btn btn-secondary" onclick="closeCustomDialog(null)">${escapeHTML(cancelText)}</button>
			<button type="button" class="btn btn-primary" onclick="submitCustomPrompt()">${escapeHTML(confirmText)}</button>
		`;

		backdrop.style.display = 'flex';
		setTimeout(() => {
			const input = document.getElementById('custom-dialog-prompt-input');
			if (input) {
				input.focus();
				input.select();
				input.onkeydown = (e) => {
					if (e.key === 'Enter') {
						e.preventDefault();
						submitCustomPrompt();
					} else if (e.key === 'Escape') {
						e.preventDefault();
						closeCustomDialog(null);
					}
				};
			}
		}, 50);
	});
}

function submitCustomPrompt() {
	const input = document.getElementById('custom-dialog-prompt-input');
	const val = input ? input.value : '';
	closeCustomDialog(val);
}

// Helpers
function formatDuration(sec) {
	if (!sec || sec <= 0) return '0:00';
	const h = Math.floor(sec / 3600);
	const m = Math.floor((sec % 3600) / 60);
	const s = sec % 60;
	if (h > 0) {
		return `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
	}
	return `${m}:${s.toString().padStart(2, '0')}`;
}

function escapeHTML(str) {
	return (str || '').replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;");
}

function showToast(message, type = 'info', duration = 4000) {
	const container = document.getElementById('toast-container');
	if (!container) return;

	const toast = document.createElement('div');
	const safeType = ['success', 'error', 'danger', 'warning', 'info'].includes(type) ? type : 'info';
	const normalizedType = safeType === 'danger' ? 'error' : safeType;
	toast.className = `toast toast-${normalizedType}`;

	let iconSVG = '';
	if (normalizedType === 'success') {
		iconSVG = '<svg viewBox="0 0 24 24"><path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/></svg>';
	} else if (normalizedType === 'error') {
		iconSVG = '<svg viewBox="0 0 24 24"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-2h2v2zm0-4h-2V7h2v6z"/></svg>';
	} else if (normalizedType === 'warning') {
		iconSVG = '<svg viewBox="0 0 24 24"><path d="M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z"/></svg>';
	} else {
		iconSVG = '<svg viewBox="0 0 24 24"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-6h2v6zm0-8h-2V7h2v2z"/></svg>';
	}

	toast.innerHTML = `
		<div class="toast-indicator">
			${iconSVG}
		</div>
		<div class="toast-content">
			<div class="toast-message">${escapeHTML(message)}</div>
		</div>
		<button class="toast-close" type="button" aria-label="Dismiss">
			<svg viewBox="0 0 24 24"><path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/></svg>
		</button>
		<div class="toast-progress" style="animation-duration: ${duration}ms;"></div>
	`;

	container.appendChild(toast);

	let timeoutId = null;
	let startTime = Date.now();
	let remainingTime = duration;
	let isPaused = false;

	const dismiss = () => {
		if (toast.classList.contains('toast-exit')) return;
		toast.classList.add('toast-exit');
		setTimeout(() => toast.remove(), 260);
	};

	const startTimer = () => {
		startTime = Date.now();
		timeoutId = setTimeout(dismiss, remainingTime);
	};

	const pauseTimer = () => {
		if (isPaused) return;
		isPaused = true;
		clearTimeout(timeoutId);
		remainingTime -= (Date.now() - startTime);
		const progress = toast.querySelector('.toast-progress');
		if (progress) progress.style.animationPlayState = 'paused';
	};

	const resumeTimer = () => {
		if (!isPaused) return;
		isPaused = false;
		const progress = toast.querySelector('.toast-progress');
		if (progress) progress.style.animationPlayState = 'running';
		startTimer();
	};

	toast.addEventListener('mouseenter', pauseTimer);
	toast.addEventListener('mouseleave', resumeTimer);

	const closeBtn = toast.querySelector('.toast-close');
	if (closeBtn) {
		closeBtn.addEventListener('click', (e) => {
			e.stopPropagation();
			clearTimeout(timeoutId);
			dismiss();
		});
	}

	startTimer();
}

// ── Native Desktop Window Controls (Wails v2) ──
function windowMinimise() {
	if (window.runtime && window.runtime.WindowMinimise) {
		window.runtime.WindowMinimise();
	} else if (window.go && window.go.main && window.go.main.App && window.go.main.App.WindowMinimise) {
		window.go.main.App.WindowMinimise();
	}
}

function windowToggleMaximise() {
	if (window.runtime && window.runtime.WindowToggleMaximise) {
		window.runtime.WindowToggleMaximise();
	} else if (window.go && window.go.main && window.go.main.App && window.go.main.App.WindowToggleMaximise) {
		window.go.main.App.WindowToggleMaximise();
	}
}

function windowClose() {
	if (window.runtime && window.runtime.Quit) {
		window.runtime.Quit();
	} else if (window.go && window.go.main && window.go.main.App && window.go.main.App.WindowClose) {
		window.go.main.App.WindowClose();
	} else {
		window.close();
	}
}

// ── High-Performance Specular Glass Cursor Tracker ──
function initLiquidGlassTracking() {
	let rafId = null;
	let currentTarget = null;
	let clientX = 0, clientY = 0;

	const updateGlassCoords = () => {
		if (currentTarget) {
			const rect = currentTarget.getBoundingClientRect();
			const x = clientX - rect.left;
			const y = clientY - rect.top;
			currentTarget.style.setProperty('--glass-x', `${x}px`);
			currentTarget.style.setProperty('--glass-y', `${y}px`);
		}
		rafId = null;
	};

	document.addEventListener('pointermove', (e) => {
		if (document.documentElement.getAttribute('data-theme') !== 'liquid-glass') return;
		
		const glassEl = e.target.closest(
			'.window-controls, .win-btn, .pref-card, .overview-card, .url-input-card, .preview-card, ' +
			'.queue-card, .library-card, .channel-card, .feature-card, .empty-state, .download-card, ' +
			'.batch-url-card, .channel-add-card, .preferences-hero, .pref-subnav, .pref-subnav-pill, ' +
			'.queue-filter-tabs, .studio-filter-tabs, .filter-pill, .library-search-box, .nav-item, ' +
			'.btn-secondary, .btn-primary, .opt-chip, .sidebar-toggle-btn, .toast, .glass-surface'
		);

		if (glassEl) {
			currentTarget = glassEl;
			clientX = e.clientX;
			clientY = e.clientY;
			if (!rafId) {
				rafId = requestAnimationFrame(updateGlassCoords);
			}
		}
	}, { passive: true });
}

// Allow double clicking on the top bar or sidebar header to toggle maximize/restore
document.addEventListener('DOMContentLoaded', () => {
	initLiquidGlassTracking();
	const topBar = document.querySelector('.top-bar');
	if (topBar) {
		topBar.addEventListener('dblclick', (e) => {
			if (e.target.closest('button, input, select, textarea, a, .window-controls, .opt-chip, .custom-select-wrap')) return;
			windowToggleMaximise();
		});
	}
});
