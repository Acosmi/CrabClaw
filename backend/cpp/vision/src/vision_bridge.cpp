/**
 * @file vision_bridge.cpp
 * @brief Crab Claw（蟹爪）视觉处理桥接实现
 *
 * 实现 vision_bridge.h 中定义的 extern "C" 接口。
 * Phase 8 中将集成 OpenCV 进行实际图像处理。
 */

#include "vision_bridge.h"
#include <cstdlib>
#include <cstring>

namespace acosmi {
namespace vision {

// 引擎初始化状态
static bool g_initialized = false;

} // namespace vision
} // namespace acosmi

extern "C" {

int acosmi_vision_init(void) {
  if (acosmi::vision::g_initialized) {
    return 0; // 已初始化
  }
  // TODO: Phase 8 — 初始化 OpenCV 和视觉处理管线
  acosmi::vision::g_initialized = true;
  return 0;
}

AcosmiResult acosmi_process_frame(const AcosmiFrame *input,
                                  AcosmiFrame *output) {
  AcosmiResult result;
  result.code = 0;
  result.message = nullptr;
  result.result_json = nullptr;

  if (!acosmi::vision::g_initialized) {
    result.code = -1;
    const char *msg = "视觉引擎未初始化";
    result.message = static_cast<char *>(malloc(strlen(msg) + 1));
    strcpy(result.message, msg);
    return result;
  }

  if (input == nullptr) {
    result.code = -2;
    const char *msg = "输入帧为空";
    result.message = static_cast<char *>(malloc(strlen(msg) + 1));
    strcpy(result.message, msg);
    return result;
  }

  // TODO: Phase 8 — 实际帧处理逻辑
  // 当前为直通模式（pass-through）
  if (output != nullptr) {
    output->width = input->width;
    output->height = input->height;
    output->channels = input->channels;
    output->stride = input->stride;
    size_t data_size = static_cast<size_t>(input->height) * input->stride;
    output->data = static_cast<unsigned char *>(malloc(data_size));
    if (output->data != nullptr && input->data != nullptr) {
      memcpy(output->data, input->data, data_size);
    }
  }

  const char *ok_msg = "处理完成";
  result.message = static_cast<char *>(malloc(strlen(ok_msg) + 1));
  strcpy(result.message, ok_msg);

  return result;
}

void acosmi_free_frame(AcosmiFrame *frame) {
  if (frame != nullptr && frame->data != nullptr) {
    free(frame->data);
    frame->data = nullptr;
  }
}

void acosmi_free_string(char *str) {
  if (str != nullptr) {
    free(str);
  }
}

void acosmi_vision_shutdown(void) {
  // TODO: Phase 8 — 释放 OpenCV 资源
  acosmi::vision::g_initialized = false;
}

} // extern "C"
