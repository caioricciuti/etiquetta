(function () {
	"use strict";

	var cfg = window.etiquettaDashboard;
	if (!cfg) return;

	var widget = document.getElementById("etiquetta-widget");
	if (!widget) return;

	var loading = widget.querySelector(".etiquetta-loading");
	var stats = widget.querySelector(".etiquetta-stats");
	var errorEl = widget.querySelector(".etiquetta-error");

	function formatNumber(n) {
		if (n == null) return "—";
		return Number(n).toLocaleString();
	}

	function formatDuration(seconds) {
		if (seconds == null || seconds === 0) return "0s";
		var s = Math.round(seconds);
		if (s < 60) return s + "s";
		var m = Math.floor(s / 60);
		var r = s % 60;
		return m + "m " + r + "s";
	}

	function formatPercent(n) {
		if (n == null) return "—";
		return Math.round(n * 100) / 100 + "%";
	}

	function showError(msg) {
		loading.style.display = "none";
		stats.style.display = "none";
		errorEl.style.display = "block";
		errorEl.textContent = msg;
	}

	function render(data) {
		loading.style.display = "none";
		stats.style.display = "block";

		var overview = data.overview || {};

		document.getElementById("etq-visitors").textContent = formatNumber(
			overview.unique_visitors || overview.visitors
		);
		document.getElementById("etq-pageviews").textContent = formatNumber(
			overview.total_pageviews || overview.pageviews
		);
		document.getElementById("etq-bounce").textContent = formatPercent(
			overview.bounce_rate
		);
		document.getElementById("etq-duration").textContent = formatDuration(
			overview.avg_session_duration || overview.avg_duration
		);

		// Top pages
		var pages = data.pages || [];
		if (Array.isArray(pages) && pages.length > 0) {
			// Handle both {data: [...]} and [...] response shapes
			if (pages.data) pages = pages.data;

			var tbody = document.getElementById("etq-top-pages");
			tbody.innerHTML = "";
			var items = pages.slice(0, 5);
			for (var i = 0; i < items.length; i++) {
				var row = document.createElement("tr");
				var pathCell = document.createElement("td");
				pathCell.textContent = items[i].path || items[i].page || "—";
				var viewsCell = document.createElement("td");
				viewsCell.textContent = formatNumber(
					items[i].visitors || items[i].views || items[i].count
				);
				row.appendChild(pathCell);
				row.appendChild(viewsCell);
				tbody.appendChild(row);
			}
		}
	}

	// Fetch stats via WP AJAX proxy
	var xhr = new XMLHttpRequest();
	xhr.open(
		"POST",
		cfg.ajaxUrl + "?action=etiquetta_stats&nonce=" + cfg.nonce,
		true
	);
	xhr.setRequestHeader("Content-Type", "application/x-www-form-urlencoded");
	xhr.onreadystatechange = function () {
		if (xhr.readyState !== 4) return;
		if (xhr.status !== 200) {
			showError("Failed to load stats (HTTP " + xhr.status + ")");
			return;
		}
		try {
			var resp = JSON.parse(xhr.responseText);
			if (resp.success) {
				render(resp.data);
			} else {
				showError(resp.data || "Unknown error");
			}
		} catch (e) {
			showError("Invalid response from server");
		}
	};
	xhr.send();
})();
