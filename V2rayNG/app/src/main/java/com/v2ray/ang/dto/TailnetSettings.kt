package com.v2ray.ang.dto

/**
 * User settings of the embedded tailnet node, shared by the settings page and the core config builder.
 * [authKey] is a one-time secret: it is cleared as soon as the node is registered.
 */
data class TailnetSettings(
    val enabled: Boolean = false,
    val controlUrl: String = "",
    val authKey: String = "",
    val hostname: String = "",
    val controlProxy: String = "",
    val magicDns: Boolean = true,
    /** Domain suffixes (for example `example.net`) that are resolved and dialed through the tailnet. */
    val domains: List<String> = emptyList(),
    /** CIDR ranges routed into the tailnet. */
    val subnets: List<String> = listOf(DEFAULT_SUBNET),
) {
    companion object {
        const val DEFAULT_SUBNET = "100.64.0.0/10"
    }
}
