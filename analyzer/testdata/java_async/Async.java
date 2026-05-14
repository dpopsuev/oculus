import java.util.concurrent.CompletableFuture;
public class Async {
    public CompletableFuture<String> fetchData() {
        return CompletableFuture.supplyAsync(() -> "data");
    }
    public void run() {
        fetchData().thenApply(s -> s);
    }
}
