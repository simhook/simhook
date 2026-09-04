package dev.simhook.app.ui.home

import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Email
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
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
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import dev.simhook.app.AppContainer

private enum class Tab(val label: String) { Dashboard("Home"), Messages("Messages"), Settings("Settings") }

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
        snackbarHost = { SnackbarHost(snackbar) },
        bottomBar = {
            NavigationBar {
                Tab.entries.forEachIndexed { index, t ->
                    NavigationBarItem(
                        selected = tab == index,
                        onClick = { tab = index },
                        icon = {
                            Icon(
                                imageVector = when (t) {
                                    Tab.Dashboard -> Icons.Filled.Home
                                    Tab.Messages -> Icons.Filled.Email
                                    Tab.Settings -> Icons.Filled.Settings
                                },
                                contentDescription = t.label,
                            )
                        },
                        label = { Text(t.label) },
                    )
                }
            }
        },
    ) { padding ->
        when (Tab.entries[tab]) {
            Tab.Dashboard -> DashboardScreen(vm, permissionsOk, Modifier.padding(padding))
            Tab.Messages -> MessagesScreen(messagesVm, Modifier.padding(padding))
            Tab.Settings -> SettingsScreen(vm, Modifier.padding(padding))
        }
    }
}
