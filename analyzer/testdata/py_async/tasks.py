import asyncio

async def fetch(url: str) -> str:
    return url

async def process(url: str) -> str:
    data = await fetch(url)   # await_call → fetch
    return data

async def run_all(urls: list) -> None:
    tasks = [fetch(u) for u in urls]
    asyncio.gather(fetch)     # task_spawn → fetch
