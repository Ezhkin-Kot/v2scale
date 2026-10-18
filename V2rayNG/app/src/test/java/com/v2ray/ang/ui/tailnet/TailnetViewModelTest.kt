package com.v2ray.ang.ui.tailnet

import android.app.Application
import com.v2ray.ang.core.TailnetConfigBuilder
import com.v2ray.ang.dto.TailnetSettings
import com.v2ray.ang.dto.TailnetStatus
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.mockito.kotlin.mock

class TailnetViewModelTest {
    private class FakeStore(var stored: TailnetSettings = TailnetSettings()) : TailnetSettingsStore {
        var saves = 0
        override fun load() = stored
        override fun save(settings: TailnetSettings) {
            saves++
            stored = settings
        }

        override fun readStatus() = TailnetStatus()
    }

    private fun viewModel(store: FakeStore) = TailnetViewModel(mock<Application>(), store)

    @Test
    fun startsWithDefaultsBeforeLoading() {
        val vm = viewModel(FakeStore())
        assertFalse(vm.form.value.enabled)
        assertEquals("100.64.0.0/10", vm.form.value.subnets)
        assertNull(vm.problem.value)
    }

    @Test
    fun loadsStoredSettingsIntoTheForm() = runBlocking {
        val vm = viewModel(FakeStore(TailnetSettings(enabled = true, controlUrl = "https://hs.example.net", domains = listOf("a.net", "b.net"))))
        vm.loadForm()
        assertTrue(vm.form.value.enabled)
        assertEquals("https://hs.example.net", vm.form.value.controlUrl)
        assertEquals("a.net\nb.net", vm.form.value.domains)
    }

    @Test
    fun emptyStoreLoadsEmptyForm() = runBlocking {
        val vm = viewModel(FakeStore())
        vm.loadForm()
        assertEquals(TailnetFormState.from(TailnetSettings()), vm.form.value)
    }

    @Test
    fun invalidSettingsAreRejectedAndNotStored() = runBlocking {
        val store = FakeStore()
        val vm = viewModel(store)
        vm.edit { it.copy(enabled = true, controlUrl = "nonsense") }
        assertFalse(vm.commit())
        assertEquals(TailnetConfigBuilder.Problem.CONTROL_URL, vm.problem.value)
        assertEquals(0, store.saves)
    }

    @Test
    fun editingClearsTheProblem() = runBlocking {
        val vm = viewModel(FakeStore())
        vm.edit { it.copy(enabled = true, controlUrl = "x") }
        vm.commit()
        vm.edit { it.copy(controlUrl = "https://hs.example.net") }
        assertNull(vm.problem.value)
    }

    @Test
    fun validSettingsAreStoredWithParsedLists() = runBlocking {
        val store = FakeStore()
        val vm = viewModel(store)
        vm.edit { it.copy(enabled = true, controlUrl = " https://hs.example.net ", domains = "a.net, b.net\na.net", authKey = " k ") }
        assertTrue(vm.commit())
        assertEquals(1, store.saves)
        assertEquals("https://hs.example.net", store.stored.controlUrl)
        assertEquals(listOf("a.net", "b.net"), store.stored.domains)
        assertEquals("k", store.stored.authKey)
    }

    @Test
    fun disabledFormSavesEvenWithBlankUrl() = runBlocking {
        val store = FakeStore(TailnetSettings(enabled = true, controlUrl = "https://hs.example.net"))
        val vm = viewModel(store)
        vm.loadForm()
        vm.edit { it.copy(enabled = false, controlUrl = "") }
        assertTrue(vm.commit())
        assertFalse(store.stored.enabled)
    }

    @Test
    fun formRoundTripKeepsSettings() {
        val s = TailnetSettings(true, "https://hs.example.net", "", "phone", "http://127.0.0.1:4444", false, listOf("a.net"), listOf("10.0.0.0/8"))
        assertEquals(s, TailnetFormState.from(s).toSettings())
    }
}
