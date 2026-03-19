<?php
/**
 * Etiquetta Dashboard Widget — shows key stats in wp-admin.
 */

if ( ! defined( 'ABSPATH' ) ) {
	exit;
}

final class Etiquetta_Dashboard_Widget {

	private static ?self $instance = null;

	public static function instance(): self {
		if ( null === self::$instance ) {
			self::$instance = new self();
		}
		return self::$instance;
	}

	/**
	 * Register the dashboard widget.
	 */
	public function register(): void {
		$server_url = get_option( 'etiquetta_server_url', '' );
		$api_key    = get_option( 'etiquetta_api_key', '' );

		if ( empty( $server_url ) || empty( $api_key ) ) {
			return;
		}

		wp_add_dashboard_widget(
			'etiquetta_dashboard_widget',
			__( 'Etiquetta Analytics', 'etiquetta-analytics' ),
			[ $this, 'render' ]
		);

		// Enqueue assets only on the dashboard
		$screen = get_current_screen();
		if ( $screen && $screen->id === 'dashboard' ) {
			wp_enqueue_style(
				'etiquetta-dashboard',
				ETIQUETTA_PLUGIN_URL . 'assets/dashboard-widget.css',
				[],
				ETIQUETTA_VERSION
			);
			wp_enqueue_script(
				'etiquetta-dashboard',
				ETIQUETTA_PLUGIN_URL . 'assets/dashboard-widget.js',
				[],
				ETIQUETTA_VERSION,
				true
			);
			wp_localize_script( 'etiquetta-dashboard', 'etiquettaDashboard', [
				'ajaxUrl' => admin_url( 'admin-ajax.php' ),
				'nonce'   => wp_create_nonce( 'etiquetta_stats' ),
			] );
		}
	}

	/**
	 * Render the widget HTML (skeleton — JS fills the data).
	 */
	public function render(): void {
		?>
		<div id="etiquetta-widget">
			<div class="etiquetta-loading"><?php esc_html_e( 'Loading stats...', 'etiquetta-analytics' ); ?></div>

			<div class="etiquetta-stats" style="display:none;">
				<div class="etiquetta-grid">
					<div class="etiquetta-stat">
						<span class="etiquetta-stat-value" id="etq-visitors">—</span>
						<span class="etiquetta-stat-label"><?php esc_html_e( 'Visitors', 'etiquetta-analytics' ); ?></span>
					</div>
					<div class="etiquetta-stat">
						<span class="etiquetta-stat-value" id="etq-pageviews">—</span>
						<span class="etiquetta-stat-label"><?php esc_html_e( 'Pageviews', 'etiquetta-analytics' ); ?></span>
					</div>
					<div class="etiquetta-stat">
						<span class="etiquetta-stat-value" id="etq-bounce">—</span>
						<span class="etiquetta-stat-label"><?php esc_html_e( 'Bounce Rate', 'etiquetta-analytics' ); ?></span>
					</div>
					<div class="etiquetta-stat">
						<span class="etiquetta-stat-value" id="etq-duration">—</span>
						<span class="etiquetta-stat-label"><?php esc_html_e( 'Avg. Duration', 'etiquetta-analytics' ); ?></span>
					</div>
				</div>

				<h4><?php esc_html_e( 'Top Pages (7 days)', 'etiquetta-analytics' ); ?></h4>
				<table class="etiquetta-table">
					<thead>
						<tr>
							<th><?php esc_html_e( 'Page', 'etiquetta-analytics' ); ?></th>
							<th><?php esc_html_e( 'Views', 'etiquetta-analytics' ); ?></th>
						</tr>
					</thead>
					<tbody id="etq-top-pages">
						<tr><td colspan="2">—</td></tr>
					</tbody>
				</table>
			</div>

			<div class="etiquetta-error" style="display:none;"></div>

			<p class="etiquetta-footer">
				<a href="<?php echo esc_url( get_option( 'etiquetta_server_url', '#' ) ); ?>" target="_blank" rel="noopener">
					<?php esc_html_e( 'Open full dashboard', 'etiquetta-analytics' ); ?> &rarr;
				</a>
			</p>
		</div>
		<?php
	}

	/**
	 * AJAX proxy — fetches stats from the Etiquetta server.
	 * Avoids CORS issues by proxying through WordPress.
	 */
	public function ajax_stats(): void {
		check_ajax_referer( 'etiquetta_stats', 'nonce' );

		if ( ! current_user_can( 'manage_options' ) ) {
			wp_send_json_error( 'Unauthorized', 403 );
		}

		$server_url = rtrim( get_option( 'etiquetta_server_url', '' ), '/' );
		$api_key    = get_option( 'etiquetta_api_key', '' );

		if ( empty( $server_url ) || empty( $api_key ) ) {
			wp_send_json_error( 'Etiquetta is not configured. Go to Settings > Etiquetta.' );
		}

		$domain = wp_parse_url( home_url(), PHP_URL_HOST );
		$now    = time() * 1000; // Etiquetta uses milliseconds
		$from   = ( time() - 7 * DAY_IN_SECONDS ) * 1000;

		$headers = [
			'Authorization' => 'Bearer ' . $api_key,
			'Accept'        => 'application/json',
		];

		// Fetch overview stats
		$overview_url = add_query_arg( [
			'domain' => $domain,
			'from'   => $from,
			'to'     => $now,
		], $server_url . '/api/stats/overview' );

		$overview = wp_remote_get( $overview_url, [
			'headers' => $headers,
			'timeout' => 10,
		] );

		if ( is_wp_error( $overview ) ) {
			wp_send_json_error( 'Could not connect to Etiquetta: ' . $overview->get_error_message() );
		}

		$overview_code = wp_remote_retrieve_response_code( $overview );
		if ( $overview_code !== 200 ) {
			wp_send_json_error( 'Etiquetta returned HTTP ' . $overview_code . '. Check your server URL and API key.' );
		}

		$overview_data = json_decode( wp_remote_retrieve_body( $overview ), true );

		// Fetch top pages
		$pages_url = add_query_arg( [
			'domain' => $domain,
			'from'   => $from,
			'to'     => $now,
			'limit'  => 5,
		], $server_url . '/api/stats/pages' );

		$pages = wp_remote_get( $pages_url, [
			'headers' => $headers,
			'timeout' => 10,
		] );

		$pages_data = [];
		if ( ! is_wp_error( $pages ) && wp_remote_retrieve_response_code( $pages ) === 200 ) {
			$pages_data = json_decode( wp_remote_retrieve_body( $pages ), true );
		}

		wp_send_json_success( [
			'overview' => $overview_data,
			'pages'    => $pages_data,
		] );
	}
}
