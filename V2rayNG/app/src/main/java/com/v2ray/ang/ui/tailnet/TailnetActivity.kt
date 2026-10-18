package com.v2ray.ang.ui.tailnet

import android.os.Bundle
import androidx.activity.viewModels
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.v2ray.ang.R
import com.v2ray.ang.core.TailnetConfigBuilder
import com.v2ray.ang.dto.TailnetStatus
import com.v2ray.ang.extension.toastSuccess
import com.v2ray.ang.ui.base.BaseComponentActivity
import com.v2ray.ang.ui.compose.AppTopBar
import com.v2ray.ang.ui.compose.FormTextField
import com.v2ray.ang.ui.compose.NavigationBarsSpacer
import com.v2ray.ang.ui.compose.SettingsSwitchItem
import com.v2ray.ang.util.Utils

/** Global Tailnet page: one embedded node shared by every proxy profile. */
class TailnetActivity : BaseComponentActivity() {
    private val viewModel: TailnetViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        if (savedInstanceState == null) viewModel.load()
    }

    @Composable
    override fun ScreenContent() {
        TailnetScreen(
            viewModel = viewModel,
            onBackClick = { finish() },
            onSave = { viewModel.save { toastSuccess(R.string.tailnet_saved) } },
            onOpenLogin = { Utils.openUri(this, it) },
        )
    }
}

@Composable
fun TailnetScreen(
    viewModel: TailnetViewModel,
    onBackClick: () -> Unit,
    onSave: () -> Unit,
    onOpenLogin: (String) -> Unit,
) {
    val form by viewModel.form.collectAsStateWithLifecycle()
    val problem by viewModel.problem.collectAsStateWithLifecycle()
    val status by viewModel.status.collectAsStateWithLifecycle()

    Scaffold(
        contentWindowInsets = WindowInsets(0),
        topBar = {
            AppTopBar(
                title = stringResource(R.string.title_tailnet),
                onBackClick = onBackClick,
                actions = {
                    IconButton(onClick = onSave) {
                        Icon(
                            painterResource(R.drawable.ic_fab_check),
                            contentDescription = stringResource(R.string.acc_save)
                        )
                    }
                }
            )
        }
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .consumeWindowInsets(innerPadding)
                .imePadding()
                .verticalScroll(rememberScrollState())
                .padding(vertical = 8.dp)
        ) {
            SettingsSwitchItem(
                title = stringResource(R.string.tailnet_enable),
                summary = stringResource(R.string.tailnet_enable_summary),
                checked = form.enabled,
                onCheckedChange = { value -> viewModel.edit { it.copy(enabled = value) } }
            )

            TailnetStatusRow(status, form.enabled, onOpenLogin)

            FormTextField(
                label = stringResource(R.string.tailnet_control_url),
                value = form.controlUrl,
                onValueChange = { value -> viewModel.edit { it.copy(controlUrl = value) } },
                keyboardType = KeyboardType.Uri,
                placeholder = "https://headscale.example.net",
                isError = problem == TailnetConfigBuilder.Problem.CONTROL_URL,
                supportingText = if (problem == TailnetConfigBuilder.Problem.CONTROL_URL) stringResource(R.string.tailnet_error_control_url) else null,
            )
            FormTextField(
                label = stringResource(R.string.tailnet_auth_key),
                value = form.authKey,
                onValueChange = { value -> viewModel.edit { it.copy(authKey = value) } },
                supportingText = stringResource(R.string.tailnet_auth_key_hint),
            )
            FormTextField(
                label = stringResource(R.string.tailnet_hostname),
                value = form.hostname,
                onValueChange = { value -> viewModel.edit { it.copy(hostname = value) } },
            )
            FormTextField(
                label = stringResource(R.string.tailnet_control_proxy),
                value = form.controlProxy,
                onValueChange = { value -> viewModel.edit { it.copy(controlProxy = value) } },
                keyboardType = KeyboardType.Uri,
                placeholder = "http://127.0.0.1:4444",
                isError = problem == TailnetConfigBuilder.Problem.CONTROL_PROXY,
                supportingText = if (problem == TailnetConfigBuilder.Problem.CONTROL_PROXY) stringResource(R.string.tailnet_error_control_proxy) else null,
            )
            FormTextField(
                label = stringResource(R.string.tailnet_domains),
                value = form.domains,
                onValueChange = { value -> viewModel.edit { it.copy(domains = value) } },
                placeholder = "example.net",
                isError = problem == TailnetConfigBuilder.Problem.DOMAIN,
                supportingText = if (problem == TailnetConfigBuilder.Problem.DOMAIN) stringResource(R.string.tailnet_error_domain) else stringResource(R.string.tailnet_domains_hint),
            )
            SettingsSwitchItem(
                title = stringResource(R.string.tailnet_magic_dns),
                summary = stringResource(R.string.tailnet_magic_dns_summary),
                checked = form.magicDns,
                onCheckedChange = { value -> viewModel.edit { it.copy(magicDns = value) } }
            )
            FormTextField(
                label = stringResource(R.string.tailnet_subnets),
                value = form.subnets,
                onValueChange = { value -> viewModel.edit { it.copy(subnets = value) } },
                placeholder = "100.64.0.0/10",
                isError = problem == TailnetConfigBuilder.Problem.SUBNET,
                supportingText = if (problem == TailnetConfigBuilder.Problem.SUBNET) stringResource(R.string.tailnet_error_subnet) else stringResource(R.string.tailnet_subnets_hint),
            )
            FormTextField(
                label = stringResource(R.string.tailnet_test_target),
                value = form.testTarget,
                onValueChange = { value -> viewModel.edit { it.copy(testTarget = value) } },
                placeholder = "nextcloud.example.net:443",
                isError = problem == TailnetConfigBuilder.Problem.TEST_TARGET,
                supportingText = if (problem == TailnetConfigBuilder.Problem.TEST_TARGET) stringResource(R.string.tailnet_error_test_target) else stringResource(R.string.tailnet_test_target_hint),
            )
            Text(
                text = stringResource(R.string.tailnet_apply_hint),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp)
            )
            NavigationBarsSpacer()
        }
    }
}

@Composable
private fun TailnetStatusRow(status: TailnetStatus, enabled: Boolean, onOpenLogin: (String) -> Unit) {
    val text = when {
        !enabled -> stringResource(R.string.tailnet_status_disabled)
        status.isRunning -> stringResource(R.string.tailnet_status_running, status.ips.joinToString())
        status.needsLogin -> stringResource(R.string.tailnet_status_login)
        status.state == TailnetStatus.STOPPED -> stringResource(R.string.tailnet_status_stopped)
        else -> stringResource(R.string.tailnet_status_connecting)
    }
    Column(modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp)) {
        Text(text = text, style = MaterialTheme.typography.bodyMedium)
        if (enabled && status.needsLogin) {
            TextButton(onClick = { onOpenLogin(status.authUrl) }) {
                Text(stringResource(R.string.tailnet_open_login))
            }
        }
    }
}
