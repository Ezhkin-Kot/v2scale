package com.v2ray.ang.ui.tailnet

import android.app.Application
import androidx.lifecycle.viewModelScope
import com.v2ray.ang.AppConfig
import com.v2ray.ang.core.TailnetConfigBuilder
import com.v2ray.ang.dto.TailnetSettings
import com.v2ray.ang.dto.TailnetStatus
import com.v2ray.ang.handler.MmkvManager
import com.v2ray.ang.handler.TailnetStore
import com.v2ray.ang.ui.base.BaseViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.flowOn
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/** Storage seam so the ViewModel logic runs in JVM tests without MMKV. */
interface TailnetSettingsStore {
    fun load(): TailnetSettings
    fun save(settings: TailnetSettings)
    fun readStatus(): TailnetStatus
}

private object MmkvTailnetSettingsStore : TailnetSettingsStore {
    override fun load() = TailnetStore.load()
    override fun save(settings: TailnetSettings) = TailnetStore.save(settings)
    override fun readStatus() =
        TailnetStatus.fresh(TailnetStatus.parse(MmkvManager.decodeSettingsString(AppConfig.PREF_TAILNET_STATUS)))
}

class TailnetViewModel(
    application: Application,
    private val store: TailnetSettingsStore,
) : BaseViewModel(application) {
    constructor(application: Application) : this(application, MmkvTailnetSettingsStore)

    private companion object {
        const val STATUS_POLL_MS = 1500L
    }

    private val _form = MutableStateFlow(TailnetFormState())
    val form: StateFlow<TailnetFormState> = _form.asStateFlow()

    private val _problem = MutableStateFlow<TailnetConfigBuilder.Problem?>(null)

    /** Set by a rejected save, cleared by the next edit. */
    val problem: StateFlow<TailnetConfigBuilder.Problem?> = _problem.asStateFlow()

    /** The daemon process publishes the node state to settings storage; this only polls it while the page is visible. */
    val status: StateFlow<TailnetStatus> by lazy {
        flow {
            while (true) {
                emit(store.readStatus())
                delay(STATUS_POLL_MS)
            }
        }.flowOn(Dispatchers.IO).stateIn(viewModelScope, SharingStarted.WhileSubscribed(5000), TailnetStatus())
    }

    /** Loads stored settings once, before the user can edit. */
    suspend fun loadForm() {
        val loaded = withContext(Dispatchers.IO) { TailnetFormState.from(store.load()) }
        _form.value = loaded
    }

    fun load() {
        viewModelScope.launch { loadForm() }
    }

    fun edit(transform: (TailnetFormState) -> TailnetFormState) {
        _form.value = transform(_form.value)
        _problem.value = null
    }

    /** @return true when the settings were valid and have been stored. */
    suspend fun commit(): Boolean {
        val form = _form.value
        val problem = form.problem()
        _problem.value = problem
        if (problem != null) return false
        withContext(Dispatchers.IO) { store.save(form.toSettings()) }
        return true
    }

    fun save(onSaved: () -> Unit) {
        viewModelScope.launch { if (commit()) onSaved() }
    }
}
