#include <future>
#include <thread>
std::string fetchHttp() { return "data"; }
void processResult(std::string s) { (void)s; }
void run() {
    auto f = std::async(fetchHttp);            // task_spawn → fetchHttp
    std::thread t(processResult, std::string{}); // goroutine → processResult
    (void)f;
}
