#pragma once
#include <stdint.h>
void *dj_audio_start(const char *input_uid, const char *output_uid, int32_t *error);
void dj_audio_stop(void *handle);
void dj_audio_gain(void *handle, float gain);
