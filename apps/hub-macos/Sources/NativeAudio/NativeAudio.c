#include "NativeAudio.h"
#include <AudioToolbox/AudioToolbox.h>
#include <pthread.h>
#include <stdlib.h>
#include <string.h>
#include <stdatomic.h>

// Bounded live PCM ring; never records to disk. Each direction owns its queues.
typedef struct {
    AudioQueueRef input, output;
    pthread_mutex_t lock;
    int16_t ring[8000];
    size_t read, count;
    _Atomic int stopping;
    _Atomic float gain;
} Bridge;
static void input_callback(void *context, AudioQueueRef queue, AudioQueueBufferRef buffer,
                           const AudioTimeStamp *time, UInt32 packets, const AudioStreamPacketDescription *descs) {
    Bridge *b = context;
    if (atomic_load(&b->stopping)) return;
    const int16_t *samples = buffer->mAudioData;
    size_t n = buffer->mAudioDataByteSize / 2;
    pthread_mutex_lock(&b->lock);
    for (size_t i = 0; i < n; i++) {
        if (b->count == 8000) { b->read = (b->read + 1) % 8000; b->count--; }
        b->ring[(b->read + b->count++) % 8000] = samples[i];
    }
    pthread_mutex_unlock(&b->lock);
    AudioQueueEnqueueBuffer(queue, buffer, 0, NULL);
}
static void output_callback(void *context, AudioQueueRef queue, AudioQueueBufferRef buffer) {
    Bridge *b = context;
    if (atomic_load(&b->stopping)) return;
    int16_t *samples = buffer->mAudioData;
    float gain = atomic_load(&b->gain);
    pthread_mutex_lock(&b->lock);
    // Drop excessive latency instead of replaying old speech after a stalled output.
    if (b->count > 2400) { b->read = (b->read + b->count - 1600) % 8000; b->count = 1600; }
    for (size_t i = 0; i < 320; i++) {
        samples[i] = b->count ? (int16_t)(b->ring[b->read] * gain) : 0;
        if (b->count) { b->read = (b->read + 1) % 8000; b->count--; }
    }
    pthread_mutex_unlock(&b->lock);
    buffer->mAudioDataByteSize = 640;
    AudioQueueEnqueueBuffer(queue, buffer, 0, NULL);
}
static OSStatus set_device(AudioQueueRef queue, const char *uid) {
    CFStringRef value = CFStringCreateWithCString(NULL, uid, kCFStringEncodingUTF8);
    if (!value) return -50;
    OSStatus result = AudioQueueSetProperty(queue, kAudioQueueProperty_CurrentDevice, &value, sizeof(value));
    CFRelease(value);
    return result;
}
void dj_audio_stop(void *handle) {
    Bridge *b = handle;
    if (!b) return;
    atomic_store(&b->stopping, 1);
    if (b->input) { AudioQueueStop(b->input, true); AudioQueueDispose(b->input, true); }
    if (b->output) { AudioQueueStop(b->output, true); AudioQueueDispose(b->output, true); }
    pthread_mutex_destroy(&b->lock);
    free(b);
}
void dj_audio_gain(void *handle, float gain) {
    if (handle) atomic_store(&((Bridge *)handle)->gain, gain < 0 ? 0 : gain > 1 ? 1 : gain);
}
void *dj_audio_start(const char *input_uid, const char *output_uid, int32_t *error) {
    if (!error) return NULL;
    if (!input_uid || !*input_uid || !output_uid || !*output_uid) { *error = -50; return NULL; }
    Bridge *b = calloc(1, sizeof(Bridge));
    if (!b) { *error = -108; return NULL; }
    pthread_mutex_init(&b->lock, NULL);
    atomic_init(&b->stopping, 0); atomic_init(&b->gain, 1);
    AudioStreamBasicDescription format = {8000, kAudioFormatLinearPCM,
        kLinearPCMFormatFlagIsSignedInteger | kLinearPCMFormatFlagIsPacked, 2, 1, 2, 1, 16, 0};
    OSStatus s = AudioQueueNewInput(&format, input_callback, b, NULL, NULL, 0, &b->input);
    if (!s) s = set_device(b->input, input_uid);
    if (!s) s = AudioQueueNewOutput(&format, output_callback, b, NULL, NULL, 0, &b->output);
    if (!s) s = set_device(b->output, output_uid);
    for (int i = 0; !s && i < 3; i++) {
        AudioQueueBufferRef in, out;
        s = AudioQueueAllocateBuffer(b->input, 640, &in);
        if (!s) s = AudioQueueEnqueueBuffer(b->input, in, 0, NULL);
        if (!s) s = AudioQueueAllocateBuffer(b->output, 640, &out);
        if (!s) output_callback(b, b->output, out);
    }
    if (!s) s = AudioQueueStart(b->input, NULL);
    if (!s) s = AudioQueueStart(b->output, NULL);
    if (s) { *error = s; dj_audio_stop(b); return NULL; }
    *error = 0; return b;
}
