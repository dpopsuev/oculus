import kotlinx.coroutines.*
suspend fun fetch(): String = "data"
fun run() {
    launch { fetch() }
    async { fetch() }
}
