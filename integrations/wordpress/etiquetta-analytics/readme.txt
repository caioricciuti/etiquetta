=== Etiquetta Analytics ===
Contributors: caioricciuti
Tags: analytics, privacy, self-hosted, web analytics, stats
Requires at least: 5.8
Tested up to: 6.7
Requires PHP: 7.4
Stable tag: 1.0.0
License: MIT
License URI: https://opensource.org/licenses/MIT

Privacy-first, self-hosted web analytics for WordPress. Lightweight tracker with a dashboard widget.

== Description ==

Etiquetta is a privacy-first, self-hosted web analytics platform. This plugin connects your WordPress site to your Etiquetta instance.

**Features:**

* Automatically injects the lightweight Etiquetta tracker (~2KB)
* Dashboard widget showing visitors, pageviews, bounce rate, and top pages
* Excludes logged-in admins from tracking
* Respects `wp_get_environment_type()` — skip tracking in dev/staging
* No cookies, no personal data collection, GDPR-friendly
* Proxies API requests through WordPress to avoid CORS issues

**Requirements:**

* A running Etiquetta instance ([get started](https://etiquetta.com))
* An API key (for the dashboard widget — tracking works without it)

== Installation ==

1. Upload the `etiquetta-analytics` folder to `/wp-content/plugins/`
2. Activate the plugin through the Plugins menu
3. Go to Settings > Etiquetta
4. Enter your Etiquetta server URL (e.g., `https://analytics.example.com`)
5. (Optional) Enter an API key for the dashboard widget

== Frequently Asked Questions ==

= Do I need an API key? =

Only for the dashboard widget. The tracker script works with just the server URL.

= Does this track personal data? =

No. Etiquetta is designed to be privacy-first. It does not use cookies and does not collect personally identifiable information.

= Can I exclude myself from tracking? =

Yes. By default, logged-in administrators are not tracked. You can change this in Settings > Etiquetta.

== Changelog ==

= 1.0.0 =
* Initial release
* Tracker script injection
* Settings page (server URL, API key, admin exclusion)
* Dashboard widget with overview stats and top pages
