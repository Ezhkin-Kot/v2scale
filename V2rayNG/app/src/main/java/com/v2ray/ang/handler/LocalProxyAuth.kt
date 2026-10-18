package com.v2ray.ang.handler

import com.v2ray.ang.AppConfig
import java.security.SecureRandom

/**
 * Credentials for the local SOCKS inbound that the user did not set themselves.
 *
 * The inbound listens on 127.0.0.1, which every app on the phone can reach. Unauthenticated it lets
 * any of them use the proxy and, with a tailnet configured, the tailnet as well. So while tailnet is
 * on and the user has set no credentials, random ones are generated once, kept in private settings
 * storage and used by every consumer (Xray inbound, the tunnel, speed tests). A user who sets their
 * own username and password keeps full control; with tailnet off nothing changes from upstream.
 */
object LocalProxyAuth {
    private const val LENGTH = 24
    private const val ALPHABET = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

    /** The credentials to use, or null for none. [auto] is the stored generated pair, if any. */
    fun effective(
        userUsername: String?,
        userPassword: String?,
        tailnetEnabled: Boolean,
        auto: Pair<String, String>?,
    ): Pair<String, String>? {
        if (!userUsername.isNullOrEmpty() && !userPassword.isNullOrEmpty()) return userUsername to userPassword
        return if (tailnetEnabled) auto else null
    }

    fun generate(random: SecureRandom = SecureRandom()): Pair<String, String> =
        randomToken(random) to randomToken(random)

    private fun randomToken(random: SecureRandom) =
        String(CharArray(LENGTH) { ALPHABET[random.nextInt(ALPHABET.length)] })

    // ---- storage glue, thin on purpose ----

    fun stored(): Pair<String, String>? {
        val u = MmkvManager.decodeSettingsString(AppConfig.PREF_SOCKS_AUTO_USERNAME)?.takeIf { it.isNotEmpty() }
        val p = MmkvManager.decodeSettingsString(AppConfig.PREF_SOCKS_AUTO_PASSWORD)?.takeIf { it.isNotEmpty() }
        return if (u != null && p != null) u to p else null
    }

    /** Creates the generated pair when it is needed and missing. Call from the process that builds the core config. */
    @Synchronized
    fun ensure() {
        if (!MmkvManager.decodeSettingsBool(AppConfig.PREF_TAILNET_ENABLED, false) || stored() != null) return
        val (u, p) = generate()
        MmkvManager.encodeSettings(AppConfig.PREF_SOCKS_AUTO_USERNAME, u)
        MmkvManager.encodeSettings(AppConfig.PREF_SOCKS_AUTO_PASSWORD, p)
    }

    fun tailnetEnabled() = MmkvManager.decodeSettingsBool(AppConfig.PREF_TAILNET_ENABLED, false)
}
