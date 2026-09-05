package dev.simhook.app.ui.home

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Snackbar
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import dev.simhook.app.AppContainer
import dev.simhook.app.ui.theme.Hairline
import dev.simhook.app.ui.theme.Mono
import dev.simhook.app.ui.theme.Tokens

private enum class Tab(val label: String) { Home("Home"), Messages("Messages"), Settings("Settings") }

/** The signed-in frame: a wordmark, a row of words for tabs, a rule, and the screen. */
@Composable
fun HomeScreen(container: AppContainer, permissionsOk: Boolean) {
    val context = LocalContext.current
    val appContext = context.applicationContext
    val vm: GatewayViewModel = viewModel(factory = viewModelFactory { initializer { GatewayViewModel(container, appContext) } })
    val messagesVm: MessagesViewModel = viewModel(factory = viewModelFactory { initializer { MessagesViewModel(container.api) } })
    var tab by rememberSaveable { mutableIntStateOf(0) }
    val snackbar = remember { SnackbarHostState() }
    val notice by vm.notice.collectAsStateWithLifecycle()

    LaunchedEffect(notice) {
        notice?.let {
            snackbar.showSnackbar(it)
            vm.consumeNotice()
        }
    }

    Scaffold(
        containerColor = Tokens.Bg,
        snackbarHost = {
            SnackbarHost(snackbar) { data ->
                Snackbar(snackbarData = data, shape = RectangleShape, containerColor = Tokens.Fg, contentColor = Tokens.Bg)
            }
        },
        topBar = {
            Column(
                Modifier
                    .fillMaxWidth()
                    .statusBarsPadding()
                    .padding(horizontal = 20.dp),
            ) {
                Row(
                    Modifier
                        .fillMaxWidth()
                        .padding(top = 14.dp, bottom = 12.dp),
                    verticalAlignment = Alignment.Bottom,
                    horizontalArrangement = Arrangement.spacedBy(20.dp),
                ) {
                    Text("simhook", fontFamily = Mono, fontWeight = FontWeight.Medium, fontSize = MaterialTheme.typography.titleMedium.fontSize, color = Tokens.Fg)
                    Tab.entries.forEachIndexed { index, t ->
                        val active = tab == index
                        Text(
                            t.label,
                            style = MaterialTheme.typography.bodyMedium.copy(textDecoration = if (active) TextDecoration.Underline else TextDecoration.None),
                            color = if (active) Tokens.Fg else Tokens.Muted,
                            modifier = Modifier.clickable { tab = index },
                        )
                    }
                }
                Hairline()
            }
        },
    ) { padding ->
        val screen = Modifier
            .fillMaxSize()
            .padding(padding)
        when (Tab.entries[tab]) {
            Tab.Home -> DashboardScreen(vm, permissionsOk, screen)
            Tab.Messages -> MessagesScreen(messagesVm, screen)
            Tab.Settings -> SettingsScreen(vm, screen)
        }
    }
}
