package com.v2ray.ang.core

import com.v2ray.ang.dto.TailnetSettings
import com.v2ray.ang.util.JsonUtil
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class TailnetConfigBuilderTest {
    private val base = TailnetSettings(enabled = true, controlUrl = "https://hs.example.net")

    @Test
    fun disabledSettingsAreAlwaysValid() {
        assertNull(TailnetConfigBuilder.validate(TailnetSettings(enabled = false, controlUrl = "junk")))
    }

    @Test
    fun validatesControlUrlProxyDomainsAndSubnets() {
        assertNull(TailnetConfigBuilder.validate(base))
        assertEquals(TailnetConfigBuilder.Problem.CONTROL_URL, TailnetConfigBuilder.validate(base.copy(controlUrl = "")))
        assertEquals(TailnetConfigBuilder.Problem.CONTROL_URL, TailnetConfigBuilder.validate(base.copy(controlUrl = "ftp://x.net")))
        assertEquals(TailnetConfigBuilder.Problem.CONTROL_PROXY, TailnetConfigBuilder.validate(base.copy(controlProxy = "nonsense")))
        assertNull(TailnetConfigBuilder.validate(base.copy(controlProxy = "http://127.0.0.1:4444")))
        assertEquals(TailnetConfigBuilder.Problem.DOMAIN, TailnetConfigBuilder.validate(base.copy(domains = listOf("bad domain"))))
        assertEquals(TailnetConfigBuilder.Problem.SUBNET, TailnetConfigBuilder.validate(base.copy(subnets = listOf("10.0.0.0"))))
        assertNull(TailnetConfigBuilder.validate(base.copy(domains = listOf("*.Example.net."), subnets = listOf("192.168.1.0/24"))))
    }

    @Test
    fun validatesTheOptionalTestTarget() {
        assertNull(TailnetConfigBuilder.validate(base))
        assertNull(TailnetConfigBuilder.validate(base.copy(testTarget = " Nextcloud.Example.net:443 ")))
        assertNull(TailnetConfigBuilder.validate(base.copy(testTarget = "100.64.0.2:443")))
        assertEquals(TailnetConfigBuilder.Problem.TEST_TARGET, TailnetConfigBuilder.validate(base.copy(testTarget = "nextcloud.example.net")))
        assertEquals(TailnetConfigBuilder.Problem.TEST_TARGET, TailnetConfigBuilder.validate(base.copy(testTarget = "https://x.net:443")))
    }

    @Test
    fun normalizesDomains() {
        assertEquals("example.net", TailnetConfigBuilder.normalizeDomain(" *.Example.NET. "))
        assertNull(TailnetConfigBuilder.normalizeDomain(""))
        assertNull(TailnetConfigBuilder.normalizeDomain("a/b"))
    }

    @Test
    fun splitsListsWithoutBlanksOrDuplicates() {
        assertEquals(listOf("a.net", "b.net"), TailnetConfigBuilder.splitList("a.net, b.net\n a.net ,,"))
        assertEquals(emptyList<String>(), TailnetConfigBuilder.splitList("  "))
    }

    @Test
    fun nodeJsonOmitsEmptyOptionalFields() {
        val json = JsonUtil.parseString(TailnetConfigBuilder.nodeJson(base, "/data/tailnet"))!!
        assertEquals("https://hs.example.net", json.get("control_url").asString)
        assertEquals("/data/tailnet", json.get("state_dir").asString)
        assertFalse(json.has("auth_key"))
        assertFalse(json.has("hostname"))
        assertFalse(json.has("control_proxy"))
    }

    @Test
    fun nodeJsonCarriesTrimmedValues() {
        val s = base.copy(authKey = " k ", hostname = " phone ", controlProxy = " http://127.0.0.1:4444 ")
        val json = JsonUtil.parseString(TailnetConfigBuilder.nodeJson(s, "/d"))!!
        assertEquals("k", json.get("auth_key").asString)
        assertEquals("phone", json.get("hostname").asString)
        assertEquals("http://127.0.0.1:4444", json.get("control_proxy").asString)
    }

    @Test
    fun routingAlwaysIncludesTailnetRangeFirst() {
        val rules = TailnetConfigBuilder.routingRules(base.copy(subnets = listOf("192.168.1.0/24", "100.64.0.0/10")))
        assertEquals(1, rules.size)
        assertEquals(listOf("100.64.0.0/10", "192.168.1.0/24"), rules[0].ip)
    }

    @Test
    fun routingAddsDomainRuleBeforeSubnetRule() {
        val rules = TailnetConfigBuilder.routingRules(base.copy(domains = listOf("Example.net", "example.net")))
        assertEquals(2, rules.size)
        assertEquals(listOf("domain:example.net"), rules[0].domain)
        assertNull(rules[0].ip)
        assertTrue(rules[1].ip!!.contains("100.64.0.0/10"))
    }

    @Test
    fun dnsServerNeedsMagicDnsAndDomains() {
        assertNull(TailnetConfigBuilder.dnsServer(base))
        assertNull(TailnetConfigBuilder.dnsServer(base.copy(domains = listOf("example.net"), magicDns = false)))
        val dns = TailnetConfigBuilder.dnsServer(base.copy(domains = listOf("example.net")))!!
        assertEquals("100.100.100.100", dns.address)
        assertEquals(listOf("domain:example.net"), dns.domains)
        assertTrue(dns.disableCache)
    }

    @Test
    fun blocksDnsOverTlsOnlyWhenTheCoreAnswersTailnetNames() {
        assertFalse(TailnetConfigBuilder.blocksDnsOverTls(base))
        assertFalse(TailnetConfigBuilder.blocksDnsOverTls(base.copy(domains = listOf("example.net"), magicDns = false)))
        assertTrue(TailnetConfigBuilder.blocksDnsOverTls(base.copy(domains = listOf("example.net"))))
    }
}
