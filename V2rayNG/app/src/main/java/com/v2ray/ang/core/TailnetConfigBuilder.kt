package com.v2ray.ang.core

import com.v2ray.ang.dto.TailnetSettings
import com.v2ray.ang.util.JsonUtil
import java.net.URI

/**
 * Pure translation of [TailnetSettings] into what the core needs: the node JSON for
 * `setTailnetConfig`, Xray routing rules and the MagicDNS server entry.
 * Nothing here touches Android or storage, so it is covered by JVM tests.
 */
object TailnetConfigBuilder {
    const val TAG = "tailnet"
    const val MAGIC_DNS_ADDRESS = "100.100.100.100"

    private val domainPattern = Regex("^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$")
    private val cidrPattern = Regex("^[0-9a-fA-F:.]+/\\d{1,3}$")

    /** Returns a user-facing problem key, or null when [s] can be started. */
    fun validate(s: TailnetSettings): Problem? {
        if (!s.enabled) return null
        val uri = runCatching { URI(s.controlUrl.trim()) }.getOrNull()
        if (uri == null || uri.host.isNullOrEmpty() || uri.scheme !in listOf("http", "https")) return Problem.CONTROL_URL
        if (s.controlProxy.isNotBlank()) {
            val proxy = runCatching { URI(s.controlProxy.trim()) }.getOrNull()
            if (proxy == null || proxy.host.isNullOrEmpty()) return Problem.CONTROL_PROXY
        }
        if (s.domains.any { normalizeDomain(it) == null }) return Problem.DOMAIN
        if (s.subnets.any { !cidrPattern.matches(it.trim()) }) return Problem.SUBNET
        return null
    }

    enum class Problem { CONTROL_URL, CONTROL_PROXY, DOMAIN, SUBNET }

    /** Lowercases and strips a leading `*.`/`.`; null when not a valid domain suffix. */
    fun normalizeDomain(raw: String): String? {
        val d = raw.trim().lowercase().removePrefix("*.").removePrefix(".").trimEnd('.')
        return d.takeIf { domainPattern.matches(it) }
    }

    /** Splits a free-form list (commas, spaces, newlines) and drops blanks and duplicates. */
    fun splitList(raw: String): List<String> =
        raw.split(',', ' ', '\n', '\t').map { it.trim() }.filter { it.isNotEmpty() }.distinct()

    fun nodeJson(s: TailnetSettings, stateDir: String): String {
        val o = linkedMapOf<String, String>()
        o["control_url"] = s.controlUrl.trim()
        if (s.authKey.isNotBlank()) o["auth_key"] = s.authKey.trim()
        if (s.hostname.isNotBlank()) o["hostname"] = s.hostname.trim()
        o["state_dir"] = stateDir
        if (s.controlProxy.isNotBlank()) o["control_proxy"] = s.controlProxy.trim()
        return JsonUtil.toJson(o)
    }

    private fun cleanDomains(s: TailnetSettings) = s.domains.mapNotNull { normalizeDomain(it) }.distinct()

    /** Xray `domain:` matchers for every tailnet domain. */
    fun domainMatchers(s: TailnetSettings): List<String> = cleanDomains(s).map { "domain:$it" }

    /** Subnets with the tailnet range always present, so the MagicDNS address and peers are reachable. */
    fun subnets(s: TailnetSettings): List<String> =
        (listOf(TailnetSettings.DEFAULT_SUBNET) + s.subnets.map { it.trim() }).filter { it.isNotEmpty() }.distinct()

    /** Rules that send tailnet traffic to the [TAG] outbound; they go first in the routing list. */
    fun routingRules(s: TailnetSettings): List<RuleSpec> {
        val rules = mutableListOf<RuleSpec>()
        val domains = domainMatchers(s)
        if (domains.isNotEmpty()) rules.add(RuleSpec(domain = domains, ip = null))
        rules.add(RuleSpec(domain = null, ip = subnets(s)))
        return rules
    }

    data class RuleSpec(val domain: List<String>?, val ip: List<String>?)

    /**
     * DNS entry that resolves tailnet domains through the node. `finalQuery` stops Xray from falling
     * back to public resolvers, so a tailnet name that does not exist never leaves the device.
     */
    fun dnsServer(s: TailnetSettings): DnsSpec? {
        if (!s.magicDns) return null
        val domains = domainMatchers(s)
        if (domains.isEmpty()) return null
        return DnsSpec(MAGIC_DNS_ADDRESS, domains, disableCache = true)
    }

    /**
     * Android sends DNS-over-TLS (port 853) straight to the VPN's DNS address, past the core's DNS
     * module, so tailnet names would never be asked for. Dropping that traffic makes Private DNS
     * validation fail and Android falls back to plain DNS, which the core does intercept.
     * Needed only while the core has to answer tailnet names; a strict Private DNS hostname set by
     * the user stays unresolvable, which is the documented limit.
     */
    fun blocksDnsOverTls(s: TailnetSettings): Boolean = dnsServer(s) != null

    const val DOT_PORT = "853"

    /**
     * [disableCache]: right after a network change the node briefly refuses names while it rebuilds
     * its DNS config, and a cached refusal would keep the name unresolvable. The node answers
     * locally, so skipping the core's cache costs nothing.
     */
    data class DnsSpec(val address: String, val domains: List<String>, val disableCache: Boolean)
}
