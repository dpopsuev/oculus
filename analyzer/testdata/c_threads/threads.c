#include <pthread.h>
void* worker(void* arg) { return arg; }
void run() {
    pthread_t t;
    pthread_create(&t, 0, worker, 0);
}
