<?php
/**
 * Plugin Name:       Etiquetta Analytics
 * Plugin URI:        https://etiquetta.com
 * Description:       Privacy-first, self-hosted web analytics. Adds the Etiquetta tracker to your site and shows stats in your WordPress dashboard.
 * Version:           1.0.0
 * Requires at least: 5.8
 * Requires PHP:      7.4
 * Author:            Etiquetta
 * Author URI:        https://etiquetta.com
 * License:           MIT
 * License URI:       https://opensource.org/licenses/MIT
 * Text Domain:       etiquetta-analytics
 */

if ( ! defined( 'ABSPATH' ) ) {
	exit;
}

define( 'ETIQUETTA_VERSION', '1.0.0' );
define( 'ETIQUETTA_PLUGIN_DIR', plugin_dir_path( __FILE__ ) );
define( 'ETIQUETTA_PLUGIN_URL', plugin_dir_url( __FILE__ ) );

require_once ETIQUETTA_PLUGIN_DIR . 'includes/class-settings.php';
require_once ETIQUETTA_PLUGIN_DIR . 'includes/class-dashboard-widget.php';

/**
 * Main plugin class.
 */
final class Etiquetta_Analytics {

	private static ?self $instance = null;

	public static function instance(): self {
		if ( null === self::$instance ) {
			self::$instance = new self();
		}
		return self::$instance;
	}

	private function __construct() {
		add_action( 'wp_head', [ $this, 'inject_tracker' ] );
		add_action( 'admin_init', [ Etiquetta_Settings::instance(), 'register' ] );
		add_action( 'admin_menu', [ Etiquetta_Settings::instance(), 'add_menu' ] );
		add_action( 'wp_dashboard_setup', [ Etiquetta_Dashboard_Widget::instance(), 'register' ] );
		add_action( 'wp_ajax_etiquetta_stats', [ Etiquetta_Dashboard_Widget::instance(), 'ajax_stats' ] );

		add_filter( 'plugin_action_links_' . plugin_basename( __FILE__ ), [ $this, 'settings_link' ] );
	}

	/**
	 * Inject the Etiquetta tracker script into <head>.
	 */
	public function inject_tracker(): void {
		// Don't track admin users if option is enabled
		if ( is_user_logged_in() && get_option( 'etiquetta_exclude_admins', '1' ) === '1' && current_user_can( 'manage_options' ) ) {
			return;
		}

		// Don't track in non-production environments
		if ( function_exists( 'wp_get_environment_type' ) && wp_get_environment_type() !== 'production' ) {
			$track_dev = get_option( 'etiquetta_track_dev', '0' );
			if ( $track_dev !== '1' ) {
				return;
			}
		}

		$server_url = esc_url( rtrim( get_option( 'etiquetta_server_url', '' ), '/' ) );
		if ( empty( $server_url ) ) {
			return;
		}

		printf(
			'<script defer src="%s/s.js" data-domain="%s"></script>' . "\n",
			esc_attr( $server_url ),
			esc_attr( wp_parse_url( home_url(), PHP_URL_HOST ) )
		);
	}

	/**
	 * Add "Settings" link on the Plugins page.
	 *
	 * @param array<string> $links Existing action links.
	 * @return array<string>
	 */
	public function settings_link( array $links ): array {
		$url = admin_url( 'options-general.php?page=etiquetta-analytics' );
		array_unshift( $links, '<a href="' . esc_url( $url ) . '">' . esc_html__( 'Settings', 'etiquetta-analytics' ) . '</a>' );
		return $links;
	}
}

Etiquetta_Analytics::instance();
