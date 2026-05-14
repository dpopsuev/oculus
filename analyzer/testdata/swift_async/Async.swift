func fetch() async -> String { return "data" }
func run() async {
    let data = await fetch()
    _ = data
}
