pub async fn fetch(url: &str) -> String { url.to_string() }
pub async fn run() {
    let _data = fetch("x").await;
    tokio::spawn(async { fetch("y").await });
}
