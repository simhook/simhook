package dev.simhook.app.ui.home

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import dev.simhook.app.api.ApiClient
import dev.simhook.app.api.ApiException
import dev.simhook.app.api.Message
import dev.simhook.app.ui.parseInstant
import dev.simhook.app.ui.relativeTime
import dev.simhook.app.ui.statusLabel
import dev.simhook.app.ui.statusTone
import dev.simhook.app.ui.theme.Hairline
import dev.simhook.app.ui.theme.StatusWord
import dev.simhook.app.ui.theme.TextLink
import dev.simhook.app.ui.theme.Tokens
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class MessagesState(
    val filter: String = "",
    val items: List<Message> = emptyList(),
    val nextCursor: String? = null,
    val loading: Boolean = false,
    val error: String? = null,
    val loadedOnce: Boolean = false,
)

class MessagesViewModel(private val api: ApiClient) : ViewModel() {
    private val _state = MutableStateFlow(MessagesState())
    val state: StateFlow<MessagesState> = _state

    fun setFilter(filter: String) {
        if (filter == _state.value.filter && _state.value.loadedOnce) return
        _state.update { it.copy(filter = filter, items = emptyList(), nextCursor = null) }
        load(reset = true)
    }

    fun refresh() = load(reset = true)

    fun loadMore() {
        val s = _state.value
        if (s.loading || s.nextCursor == null) return
        load(reset = false)
    }

    private fun load(reset: Boolean) {
        val s = _state.value
        if (s.loading) return
        _state.update { it.copy(loading = true, error = null) }
        viewModelScope.launch {
            try {
                val page = api.messages(direction = s.filter.ifEmpty { null }, cursor = if (reset) null else s.nextCursor)
                _state.update {
                    it.copy(
                        items = if (reset) page.data else it.items + page.data,
                        nextCursor = page.nextCursor,
                        loading = false,
                        loadedOnce = true,
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(loading = false, error = e.message, loadedOnce = true) }
            } catch (e: Exception) {
                _state.update { it.copy(loading = false, error = "Could not reach the server.", loadedOnce = true) }
            }
        }
    }
}

@Composable
fun MessagesScreen(vm: MessagesViewModel, modifier: Modifier = Modifier) {
    val state by vm.state.collectAsStateWithLifecycle()
    LaunchedEffect(Unit) { if (!state.loadedOnce) vm.refresh() }

    Column(modifier = modifier.fillMaxSize()) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 20.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(18.dp),
        ) {
            // Filters are words; the current one is underlined.
            val options = listOf("" to "All", "outbound" to "Sent", "inbound" to "Received")
            options.forEach { (value, label) ->
                val active = state.filter == value
                Text(
                    label,
                    style = MaterialTheme.typography.bodyMedium.copy(textDecoration = if (active) TextDecoration.Underline else TextDecoration.None),
                    color = if (active) Tokens.Fg else Tokens.Muted,
                    modifier = Modifier.clickable { vm.setFilter(value) },
                )
            }
            Spacer(Modifier.weight(1f))
            TextLink("Refresh", onClick = vm::refresh, enabled = !state.loading)
        }
        Hairline(Modifier.padding(horizontal = 20.dp))
        state.error?.let {
            Text(it, color = Tokens.Bad, modifier = Modifier.padding(horizontal = 20.dp, vertical = 10.dp), style = MaterialTheme.typography.bodyMedium)
        }
        if (state.items.isEmpty()) {
            Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Text(if (state.loading) "Loading…" else "No messages yet.", color = Tokens.Muted, style = MaterialTheme.typography.bodyMedium)
            }
            return
        }
        LazyColumn(Modifier.fillMaxSize()) {
            itemsIndexed(state.items, key = { _, m -> m.id }) { index, message ->
                if (index >= state.items.size - 5) LaunchedEffect(state.items.size) { vm.loadMore() }
                MessageRow(message)
                Hairline(Modifier.padding(horizontal = 20.dp))
            }
            if (state.loading) {
                itemsIndexed(listOf(Unit)) { _, _ ->
                    Box(Modifier.fillMaxWidth().padding(16.dp), contentAlignment = Alignment.Center) {
                        Text("Loading…", color = Tokens.Muted, style = MaterialTheme.typography.bodySmall)
                    }
                }
            }
        }
    }
}

@Composable
private fun MessageRow(m: Message) {
    val outbound = m.direction == "outbound"
    val other = (if (outbound) m.recipient else m.sender) ?: "unknown"
    val at = parseInstant(if (outbound) m.createdAt else (m.receivedAt ?: m.createdAt)) ?: 0L
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 20.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(3.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            Text(if (outbound) "to" else "from", style = MaterialTheme.typography.labelSmall, color = Tokens.Muted)
            Text(other, style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
            Text(relativeTime(at), style = MaterialTheme.typography.labelSmall, color = Tokens.Muted)
        }
        Text(m.body, style = MaterialTheme.typography.bodyMedium, maxLines = 2, overflow = TextOverflow.Ellipsis)
        StatusWord(statusTone(m.status), statusLabel(m.status) + (m.errorMessage?.let { ", $it" } ?: ""), muted = true)
    }
}
