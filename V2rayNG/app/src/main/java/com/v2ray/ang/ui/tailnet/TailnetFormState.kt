package com.v2ray.ang.ui.tailnet

import com.v2ray.ang.core.TailnetConfigBuilder
import com.v2ray.ang.dto.TailnetSettings

/** Text-level state of the Tailnet page; lists are edited as free text, one entry per line. */
data class TailnetFormState(
    val enabled: Boolean = false,
    val controlUrl: String = "",
    val authKey: String = "",
    val hostname: String = "",
    val controlProxy: String = "",
    val magicDns: Boolean = true,
    val domains: String = "",
    val subnets: String = TailnetSettings.DEFAULT_SUBNET,
) {
    fun toSettings() = TailnetSettings(
        enabled = enabled,
        controlUrl = controlUrl.trim(),
        authKey = authKey.trim(),
        hostname = hostname.trim(),
        controlProxy = controlProxy.trim(),
        magicDns = magicDns,
        domains = TailnetConfigBuilder.splitList(domains),
        subnets = TailnetConfigBuilder.splitList(subnets),
    )

    fun problem(): TailnetConfigBuilder.Problem? = TailnetConfigBuilder.validate(toSettings())

    companion object {
        fun from(s: TailnetSettings) = TailnetFormState(
            enabled = s.enabled,
            controlUrl = s.controlUrl,
            authKey = s.authKey,
            hostname = s.hostname,
            controlProxy = s.controlProxy,
            magicDns = s.magicDns,
            domains = s.domains.joinToString("\n"),
            subnets = s.subnets.joinToString("\n"),
        )
    }
}
