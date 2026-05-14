using System.Threading.Tasks;
public class Async {
    public async Task<string> FetchAsync() { return await GetData(); }
    private Task<string> GetData() { return Task.Run(() => "data"); }
}
