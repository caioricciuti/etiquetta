<?php
/**
 * Etiquetta Settings Page.
 */

if ( ! defined( 'ABSPATH' ) ) {
	exit;
}

final class Etiquetta_Settings {

	private static ?self $instance = null;

	public static function instance(): self {
		if ( null === self::$instance ) {
			self::$instance = new self();
		}
		return self::$instance;
	}

	/**
	 * Register settings fields.
	 */
	public function register(): void {
		register_setting( 'etiquetta_settings', 'etiquetta_server_url', [
			'type'              => 'string',
			'sanitize_callback' => 'esc_url_raw',
			'default'           => '',
		] );

		register_setting( 'etiquetta_settings', 'etiquetta_api_key', [
			'type'              => 'string',
			'sanitize_callback' => 'sanitize_text_field',
			'default'           => '',
		] );

		register_setting( 'etiquetta_settings', 'etiquetta_exclude_admins', [
			'type'              => 'string',
			'sanitize_callback' => 'sanitize_text_field',
			'default'           => '1',
		] );

		register_setting( 'etiquetta_settings', 'etiquetta_track_dev', [
			'type'              => 'string',
			'sanitize_callback' => 'sanitize_text_field',
			'default'           => '0',
		] );

		add_settings_section(
			'etiquetta_main',
			__( 'Connection', 'etiquetta-analytics' ),
			'__return_null',
			'etiquetta-analytics'
		);

		add_settings_field( 'etiquetta_server_url', __( 'Server URL', 'etiquetta-analytics' ), [ $this, 'field_server_url' ], 'etiquetta-analytics', 'etiquetta_main' );
		add_settings_field( 'etiquetta_api_key', __( 'API Key', 'etiquetta-analytics' ), [ $this, 'field_api_key' ], 'etiquetta-analytics', 'etiquetta_main' );

		add_settings_section(
			'etiquetta_tracking',
			__( 'Tracking', 'etiquetta-analytics' ),
			'__return_null',
			'etiquetta-analytics'
		);

		add_settings_field( 'etiquetta_exclude_admins', __( 'Exclude admins', 'etiquetta-analytics' ), [ $this, 'field_exclude_admins' ], 'etiquetta-analytics', 'etiquetta_tracking' );
		add_settings_field( 'etiquetta_track_dev', __( 'Track non-production', 'etiquetta-analytics' ), [ $this, 'field_track_dev' ], 'etiquetta-analytics', 'etiquetta_tracking' );
	}

	/**
	 * Add the settings page under Settings menu.
	 */
	public function add_menu(): void {
		add_options_page(
			__( 'Etiquetta Analytics', 'etiquetta-analytics' ),
			__( 'Etiquetta', 'etiquetta-analytics' ),
			'manage_options',
			'etiquetta-analytics',
			[ $this, 'render_page' ]
		);
	}

	/**
	 * Render the settings page.
	 */
	public function render_page(): void {
		if ( ! current_user_can( 'manage_options' ) ) {
			return;
		}
		?>
		<div class="wrap">
			<h1><?php echo esc_html( get_admin_page_title() ); ?></h1>
			<form action="options.php" method="post">
				<?php
				settings_fields( 'etiquetta_settings' );
				do_settings_sections( 'etiquetta-analytics' );
				submit_button();
				?>
			</form>
			<hr>
			<h2><?php esc_html_e( 'How to get your API Key', 'etiquetta-analytics' ); ?></h2>
			<ol>
				<li><?php esc_html_e( 'Log in to your Etiquetta dashboard.', 'etiquetta-analytics' ); ?></li>
				<li><?php esc_html_e( 'Go to Settings > API Keys.', 'etiquetta-analytics' ); ?></li>
				<li><?php esc_html_e( 'Create a new key and paste it above.', 'etiquetta-analytics' ); ?></li>
			</ol>
			<p>
				<em><?php esc_html_e( 'The API key is used only for the dashboard widget. Tracking works without it.', 'etiquetta-analytics' ); ?></em>
			</p>
		</div>
		<?php
	}

	// ── Field renderers ──────────────────────────────────

	public function field_server_url(): void {
		$value = get_option( 'etiquetta_server_url', '' );
		printf(
			'<input type="url" name="etiquetta_server_url" value="%s" class="regular-text" placeholder="https://analytics.example.com" />',
			esc_attr( $value )
		);
		echo '<p class="description">' . esc_html__( 'The URL of your Etiquetta instance (no trailing slash).', 'etiquetta-analytics' ) . '</p>';
	}

	public function field_api_key(): void {
		$value = get_option( 'etiquetta_api_key', '' );
		printf(
			'<input type="password" name="etiquetta_api_key" value="%s" class="regular-text" placeholder="etq_..." autocomplete="off" />',
			esc_attr( $value )
		);
		echo '<p class="description">' . esc_html__( 'Required for the dashboard widget. Generate one in your Etiquetta settings.', 'etiquetta-analytics' ) . '</p>';
	}

	public function field_exclude_admins(): void {
		$checked = get_option( 'etiquetta_exclude_admins', '1' ) === '1';
		printf(
			'<label><input type="checkbox" name="etiquetta_exclude_admins" value="1" %s /> %s</label>',
			checked( $checked, true, false ),
			esc_html__( 'Do not track logged-in administrators', 'etiquetta-analytics' )
		);
	}

	public function field_track_dev(): void {
		$checked = get_option( 'etiquetta_track_dev', '0' ) === '1';
		printf(
			'<label><input type="checkbox" name="etiquetta_track_dev" value="1" %s /> %s</label>',
			checked( $checked, true, false ),
			esc_html__( 'Track visits in development/staging environments', 'etiquetta-analytics' )
		);
	}
}
