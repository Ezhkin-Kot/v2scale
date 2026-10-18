package com.v2ray.ang.handler

import com.v2ray.ang.AppConfig
import com.v2ray.ang.core.TailnetConfigBuilder
import com.v2ray.ang.dto.TailnetSettings

/**
 * Persists [TailnetSettings] in the shared settings storage (MMKV, visible to the daemon process).
 * Lists are stored as one line each, split again with [TailnetConfigBuilder.splitList].
 */
object TailnetStore {
    fun load(): TailnetSettings = TailnetSettings(
        enabled = MmkvManager.decodeSettingsBool(AppConfig.PREF_TAILNET_ENABLED, false),
        controlUrl = MmkvManager.decodeSettingsString(AppConfig.PREF_TAILNET_CONTROL_URL).orEmpty(),
        authKey = MmkvManager.decodeSettingsString(AppConfig.PREF_TAILNET_AUTH_KEY).orEmpty(),
        hostname = MmkvManager.decodeSettingsString(AppConfig.PREF_TAILNET_HOSTNAME).orEmpty(),
        controlProxy = MmkvManager.decodeSettingsString(AppConfig.PREF_TAILNET_CONTROL_PROXY).orEmpty(),
        magicDns = MmkvManager.decodeSettingsBool(AppConfig.PREF_TAILNET_MAGIC_DNS, true),
        domains = TailnetConfigBuilder.splitList(MmkvManager.decodeSettingsString(AppConfig.PREF_TAILNET_DOMAINS).orEmpty()),
        subnets = MmkvManager.decodeSettingsString(AppConfig.PREF_TAILNET_SUBNETS)
            ?.let { TailnetConfigBuilder.splitList(it) }
            ?: listOf(TailnetSettings.DEFAULT_SUBNET),
        testTarget = MmkvManager.decodeSettingsString(AppConfig.PREF_TAILNET_TEST_TARGET).orEmpty(),
    )

    fun save(s: TailnetSettings) {
        MmkvManager.encodeSettings(AppConfig.PREF_TAILNET_ENABLED, s.enabled)
        MmkvManager.encodeSettings(AppConfig.PREF_TAILNET_CONTROL_URL, s.controlUrl.trim())
        MmkvManager.encodeSettings(AppConfig.PREF_TAILNET_AUTH_KEY, s.authKey.trim())
        MmkvManager.encodeSettings(AppConfig.PREF_TAILNET_HOSTNAME, s.hostname.trim())
        MmkvManager.encodeSettings(AppConfig.PREF_TAILNET_CONTROL_PROXY, s.controlProxy.trim())
        MmkvManager.encodeSettings(AppConfig.PREF_TAILNET_MAGIC_DNS, s.magicDns)
        MmkvManager.encodeSettings(AppConfig.PREF_TAILNET_DOMAINS, s.domains.joinToString("\n"))
        MmkvManager.encodeSettings(AppConfig.PREF_TAILNET_SUBNETS, s.subnets.joinToString("\n"))
        MmkvManager.encodeSettings(AppConfig.PREF_TAILNET_TEST_TARGET, s.testTarget.trim())
    }

    /** The auth key is single-use; once the node has registered it must not stay on disk. */
    fun clearAuthKey() {
        MmkvManager.encodeSettings(AppConfig.PREF_TAILNET_AUTH_KEY, "")
    }
}
