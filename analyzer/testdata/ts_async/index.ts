async function fetchData(url: string): Promise<string> {
  const resp = await fetch(url);        // await_call → fetch
  return resp.text();
}

function loadWithCallback(url: string, onData: (s: string) => void): void {
  fetchData(url).then(onData);          // promise_chain → onData
}
