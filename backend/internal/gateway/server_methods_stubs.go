package gateway

// server_methods_stubs.go — 已清空
// 所有 Batch D-W1 的 45 个方法已全量实现，分布在以下文件：
//   server_methods_cron.go      — wake, cron.* (8 方法)
//   server_methods_tts.go       — tts.* (6 方法)
//   server_methods_skills.go    — skills.* (4 方法)
//   server_methods_nodes.go     — node.* (11 方法)
//   server_methods_devices.go   — device.* (5 方法)
//   server_methods_voicewake.go — voicewake.* (2 方法)
//   server_methods_update.go    — update.* / desktop.update.* (8 方法)
//   server_methods_browser.go   — browser.request (1 方法)
//   server_methods_talk.go      — talk.mode (1 方法)
//   server_methods_web.go       — web.login.* (2 方法)
//
// 原 StubHandlers() 已删除。
// 原 exec.approval.request/resolve/list/resolve 4 个错误方法名已删除
// （TS 中对应 exec.approvals.get/set/node.get/node.set，由
//   server_methods_exec_approvals.go 的 ExecApprovalsHandlers() 注册）。
