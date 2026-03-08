/**
 * @file vision_bridge.h
 * @brief Crab Claw（蟹爪）视觉处理 C++ 桥接接口
 * 
 * 提供 extern "C" 接口供 Go CGO 调用。
 * 所有函数名以 acosmi_ 前缀命名空间。
 */

#ifndef ACOSMI_VISION_BRIDGE_H
#define ACOSMI_VISION_BRIDGE_H

#ifdef __cplusplus
extern "C" {
#endif

#include <stdint.h>

/**
 * 帧数据结构 — Go 和 C++ 之间传递图像的通用格式
 */
typedef struct {
    unsigned char* data;    /** 像素数据指针 */
    int width;              /** 宽度（像素） */
    int height;             /** 高度（像素） */
    int channels;           /** 通道数（3=RGB, 4=RGBA） */
    int stride;             /** 行步长（字节） */
} AcosmiFrame;

/**
 * 处理结果
 */
typedef struct {
    int code;               /** 状态码（0=成功） */
    char* message;          /** 状态消息（需调用 acosmi_free_string 释放） */
    char* result_json;      /** JSON 格式的处理结果（需调用 acosmi_free_string 释放） */
} AcosmiResult;

/**
 * 初始化视觉处理引擎
 * @return 0=成功, 非0=错误码
 */
int acosmi_vision_init(void);

/**
 * 处理单帧图像
 * @param input 输入帧
 * @param output 输出帧（由调用方分配内存）
 * @return AcosmiResult 处理结果
 */
AcosmiResult acosmi_process_frame(const AcosmiFrame* input, AcosmiFrame* output);

/**
 * 释放帧内部数据
 */
void acosmi_free_frame(AcosmiFrame* frame);

/**
 * 释放 C 字符串
 */
void acosmi_free_string(char* str);

/**
 * 关闭视觉处理引擎，释放所有资源
 */
void acosmi_vision_shutdown(void);

#ifdef __cplusplus
}
#endif

#endif /* ACOSMI_VISION_BRIDGE_H */
