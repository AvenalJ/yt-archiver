// YT Themer — Frontend Application Logic

let scanResults = null;
let selectedFiles = new Set();
let selectedTemplate = null;
let availableTemplates = {};

// ========== STEP 1: SCAN ==========
async function scanDirectory() {
	const input = document.getElementById('folder-input');
	const path = input.value.trim();
	if (!path) {
		showToast('Please enter a folder path', 'error');
		return;
	}

	const scanBtn = document.getElementById('scan-btn');
	const status = document.getElementById('scan-status');

	scanBtn.disabled = true;
	status.style.display = 'flex';
	status.className = 'scan-status loading';
	status.innerHTML = '<span class="spinner"></span> Scanning directory...';

	try {
		const res = await fetch('/api/scan', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ path })
		});

		if (!res.ok) {
			const err = await res.json();
			throw new Error(err.error || 'Scan failed');
		}

		scanResults = await res.json();

		if (!scanResults.files || scanResults.files.length === 0) {
			status.className = 'scan-status error';
			status.innerHTML = 'No YT Archive HTML files found in this directory.';
			scanBtn.disabled = false;
			return;
		}

		status.className = 'scan-status success';
		status.innerHTML = `Found ${scanResults.summary.total_files} YT Archive HTML files`;

		// Initialize selection (all selected by default)
		selectedFiles.clear();
		scanResults.files.forEach(f => selectedFiles.add(f.path));

		renderResults();
		await loadTemplates();

		// Show subsequent steps
		document.getElementById('step-results').style.display = 'block';
		document.getElementById('step-templates').style.display = 'block';
		document.getElementById('step-apply').style.display = 'block';

		updateApplySection();

		// Scroll to results
		document.getElementById('step-results').scrollIntoView({ behavior: 'smooth', block: 'start' });

	} catch (err) {
		status.className = 'scan-status error';
		status.innerHTML = `${err.message}`;
	} finally {
		scanBtn.disabled = false;
	}
}

// ========== STEP 2: RESULTS ==========
function renderResults() {
	if (!scanResults) return;

	const summary = scanResults.summary;
	const badgesEl = document.getElementById('summary-badges');
	badgesEl.innerHTML = `
		<span class="badge badge-total">${summary.total_files} Total</span>
		${summary.video_count ? `<span class="badge badge-video">${summary.video_count} Video Player${summary.video_count !== 1 ? 's' : ''}</span>` : ''}
		${summary.portal_count ? `<span class="badge badge-portal">${summary.portal_count} Portal${summary.portal_count !== 1 ? 's' : ''}</span>` : ''}
		${summary.channel_count ? `<span class="badge badge-channel">${summary.channel_count} Channel Page${summary.channel_count !== 1 ? 's' : ''}</span>` : ''}
	`;

	document.getElementById('results-subtitle').textContent =
		`Found ${summary.total_files} YT Archive HTML files in ${scanResults.root_path}`;

	const listEl = document.getElementById('file-list');
	listEl.innerHTML = '';

	for (const file of scanResults.files) {
		const item = document.createElement('div');
		item.className = 'file-item';
		item.onclick = (e) => {
			if (e.target.type === 'checkbox') return;
			const cb = item.querySelector('input[type="checkbox"]');
			cb.checked = !cb.checked;
			toggleFileSelection(file.path, cb.checked);
		};

		const checked = selectedFiles.has(file.path) ? 'checked' : '';
		const themeLabel = file.current_theme || 'default';
		item.innerHTML = `
			<input type="checkbox" ${checked} onchange="toggleFileSelection('${escapeHTML(file.path)}', this.checked)">
			<span class="file-type-badge ${file.page_type}">${file.page_type}</span>
			<span class="file-theme-badge ${themeLabel}">theme: ${themeLabel}</span>
			<span class="file-label">${escapeHTML(file.page_label)}</span>
			<span class="file-path" title="${escapeHTML(file.relative_path)}">${escapeHTML(file.relative_path)}</span>
		`;
		listEl.appendChild(item);
	}

	updateSelectedCount();
}

function toggleFileSelection(path, isSelected) {
	if (isSelected) {
		selectedFiles.add(path);
	} else {
		selectedFiles.delete(path);
	}
	updateSelectedCount();
	updateApplySection();

	// Update select-all checkbox state
	const selectAll = document.getElementById('select-all-checkbox');
	selectAll.checked = selectedFiles.size === scanResults.files.length;
	selectAll.indeterminate = selectedFiles.size > 0 && selectedFiles.size < scanResults.files.length;
}

function toggleSelectAll(checked) {
	if (!scanResults) return;
	selectedFiles.clear();
	if (checked) {
		scanResults.files.forEach(f => selectedFiles.add(f.path));
	}

	// Update all checkboxes
	document.querySelectorAll('.file-item input[type="checkbox"]').forEach(cb => {
		cb.checked = checked;
	});

	updateSelectedCount();
	updateApplySection();
}

function updateSelectedCount() {
	document.getElementById('selected-count').textContent = `${selectedFiles.size} selected`;
}

// ========== STEP 3: TEMPLATES ==========
async function loadTemplates() {
	try {
		const res = await fetch('/api/templates');
		availableTemplates = await res.json();
		renderTemplateGallery();
	} catch (err) {
		console.error('Failed to load templates:', err);
	}
}

function renderTemplateGallery() {
	const gallery = document.getElementById('template-gallery');
	gallery.innerHTML = '';

	// Use video templates as the display list (same styles across all categories)
	const templateList = availableTemplates.video || [];

	for (const tmpl of templateList) {
		const card = document.createElement('div');
		card.className = `template-card${selectedTemplate === tmpl.id ? ' selected' : ''}`;
		card.onclick = () => selectTemplate(tmpl.id);

		const colorsHTML = (tmpl.colors || []).map(c =>
			`<div class="color-swatch" style="background:${c}"></div>`
		).join('');

		card.innerHTML = `
			<div class="template-colors">${colorsHTML}</div>
			<div class="template-name">${escapeHTML(tmpl.name)}</div>
			<div class="template-desc">${escapeHTML(tmpl.description)}</div>
		`;
		gallery.appendChild(card);
	}
}

function selectTemplate(id) {
	selectedTemplate = id;
	renderTemplateGallery();
	updateApplySection();

	// Scroll to apply if first selection
	const applySection = document.getElementById('step-apply');
	applySection.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}

// ========== STEP 4: APPLY ==========
function updateApplySection() {
	const title = document.getElementById('apply-title');
	const desc = document.getElementById('apply-desc');
	const btn = document.getElementById('apply-btn');

	const count = selectedFiles.size;
	const tmplName = selectedTemplate ?
		(availableTemplates.video || []).find(t => t.id === selectedTemplate)?.name || selectedTemplate :
		'none';

	if (!selectedTemplate) {
		title.textContent = 'Select a Template';
		desc.textContent = `${count} file${count !== 1 ? 's' : ''} selected. Choose a template from above.`;
		btn.disabled = true;
	} else if (count === 0) {
		title.textContent = 'Select Files';
		desc.textContent = `Template: ${tmplName}. Select files from the list above to apply.`;
		btn.disabled = true;
	} else {
		title.textContent = `Ready to Apply "${tmplName}"`;
		desc.textContent = `Will apply to ${count} file${count !== 1 ? 's' : ''}. Original files will be backed up as .bak`;
		btn.disabled = false;
	}
}

async function applyTemplate() {
	if (!selectedTemplate || selectedFiles.size === 0) return;

	const btn = document.getElementById('apply-btn');
	const progress = document.getElementById('apply-progress');
	const progressBar = document.getElementById('progress-bar');
	const progressText = document.getElementById('progress-text');
	const results = document.getElementById('apply-results');

	btn.disabled = true;
	progress.style.display = 'block';
	results.style.display = 'none';
	progressBar.style.width = '0%';
	progressText.textContent = 'Applying template...';

	try {
		const files = Array.from(selectedFiles);

		const res = await fetch('/api/apply', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({
				template_id: selectedTemplate,
				files: files
			})
		});

		const data = await res.json();

		// Animate progress bar
		progressBar.style.width = '100%';

		// Show results
		results.style.display = 'block';
		results.innerHTML = '';

		let successCount = 0;
		let failCount = 0;

		for (const detail of (data.details || [])) {
			const item = document.createElement('div');
			if (detail.status === 'success') {
				successCount++;
				item.className = 'result-item success';
				item.innerHTML = `<span class="result-icon">✓</span> ${escapeHTML(detail.path.split('/').pop())} ${detail.message ? '— ' + escapeHTML(detail.message) : ''}`;
			} else {
				failCount++;
				item.className = 'result-item error';
				item.innerHTML = `<span class="result-icon">✕</span> ${escapeHTML(detail.path.split('/').pop())} — ${escapeHTML(detail.message || 'Unknown error')}`;
			}
			results.appendChild(item);
		}

		progressText.textContent = `Done! ${successCount} applied, ${failCount} failed.`;

		if (failCount === 0) {
			showToast(`Successfully applied template to ${successCount} files!`, 'success');
		} else {
			showToast(`Applied to ${successCount} files, ${failCount} failed.`, 'error');
		}

	} catch (err) {
		progressText.textContent = 'Error: ' + err.message;
		showToast('Failed to apply template: ' + err.message, 'error');
	} finally {
		btn.disabled = false;
	}
}

async function restoreAll() {
	if (selectedFiles.size === 0) {
		showToast('No files selected to restore', 'error');
		return;
	}

	const confirmed = await showConfirmDialog({
		title: 'Restore Backups?',
		message: `Restore ${selectedFiles.size} file(s) from .bak backups? This will revert any template changes to their original design.`,
		confirmText: 'Restore Backups',
		cancelText: 'Cancel',
		danger: true
	});
	if (!confirmed) return;

	try {
		const res = await fetch('/api/restore', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ files: Array.from(selectedFiles) })
		});

		const data = await res.json();
		if (data.success) {
			showToast(`Restored ${data.restored} files from backup!`, 'success');
		} else {
			showToast(`Restored ${data.restored}, failed ${data.failed}.`, 'error');
		}
	} catch (err) {
		showToast('Restore failed: ' + err.message, 'error');
	}
}

// ========== CUSTOM MODALS & DIALOGS ==========
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

// ========== UTILITIES ==========
function showToast(message, type = 'info') {
	const toast = document.getElementById('toast');
	toast.textContent = message;
	toast.className = `toast show ${type}`;
	setTimeout(() => {
		toast.className = 'toast';
	}, 3500);
}

function escapeHTML(str) {
	const div = document.createElement('div');
	div.textContent = str;
	return div.innerHTML;
}

// Initialize on DOM load
document.addEventListener('DOMContentLoaded', () => {
	loadTemplates();
	const input = document.getElementById('folder-input');
	if (input) {
		input.addEventListener('keydown', (e) => {
			if (e.key === 'Enter') {
				scanDirectory();
			}
		});
	}
});
