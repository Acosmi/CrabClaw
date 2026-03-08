package autoreply

// TS 对照: auto-reply/commands-registry.data.ts (615L)
// 完整命令定义注册，对齐 TS buildChatCommands()。

func init() {
	registerBuiltinCommands()
}

func registerBuiltinCommands() {
	// --- 状态类 (status) ---

	RegisterCommand(&ChatCommandDefinition{
		Key:         "help",
		NativeName:  "help",
		Description: "Show available commands.",
		TextAliases: []string{"/help"},
		Scope:       CommandScopeBoth,
		Category:    CategoryStatus,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "commands",
		NativeName:  "commands",
		Description: "List all slash commands.",
		TextAliases: []string{"/commands"},
		Scope:       CommandScopeBoth,
		Category:    CategoryStatus,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "status",
		NativeName:  "status",
		Description: "Show current status.",
		TextAliases: []string{"/status"},
		Scope:       CommandScopeBoth,
		Category:    CategoryStatus,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "whoami",
		NativeName:  "whoami",
		Description: "Show your sender id.",
		TextAliases: []string{"/whoami", "/id"},
		Scope:       CommandScopeBoth,
		Category:    CategoryStatus,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "context",
		NativeName:  "context",
		Description: "Explain how context is built and used.",
		TextAliases: []string{"/context"},
		AcceptsArgs: true,
		Scope:       CommandScopeBoth,
		Category:    CategoryStatus,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "usage",
		NativeName:  "usage",
		Description: "Usage footer or cost summary.",
		TextAliases: []string{"/usage"},
		AcceptsArgs: true,
		Args: []CommandArgDefinition{
			{Name: "mode", Description: "off, tokens, full, or cost", Type: ArgTypeString,
				Choices: []CommandArgChoice{
					{Value: "off", Label: "Off"},
					{Value: "tokens", Label: "Tokens"},
					{Value: "full", Label: "Full"},
					{Value: "cost", Label: "Cost"},
				}},
		},
		ArgsParsing:  ArgsParsingPositional,
		ArgsMenuAuto: true,
		Scope:        CommandScopeBoth,
		Category:     CategoryOptions,
	})

	// --- 会话类 (session) ---

	RegisterCommand(&ChatCommandDefinition{
		Key:         "stop",
		NativeName:  "stop",
		Description: "Stop the current run.",
		TextAliases: []string{"/stop"},
		Scope:       CommandScopeBoth,
		Category:    CategorySession,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "reset",
		NativeName:  "reset",
		Description: "Reset the current session.",
		TextAliases: []string{"/reset"},
		AcceptsArgs: true,
		Scope:       CommandScopeBoth,
		Category:    CategorySession,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "new",
		NativeName:  "new",
		Description: "Start a new session.",
		TextAliases: []string{"/new"},
		AcceptsArgs: true,
		Scope:       CommandScopeBoth,
		Category:    CategorySession,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "compact",
		Description: "Compact the session context.",
		TextAliases: []string{"/compact"},
		AcceptsArgs: true,
		Args: []CommandArgDefinition{
			{Name: "instructions", Description: "Extra compaction instructions", Type: ArgTypeString, CaptureRemaining: true},
		},
		ArgsParsing: ArgsParsingPositional,
		Scope:       CommandScopeText,
		Category:    CategorySession,
	})

	// --- 选项类 (options) ---

	RegisterCommand(&ChatCommandDefinition{
		Key:         "model",
		NativeName:  "model",
		Description: "Show or set the model.",
		TextAliases: []string{"/model"},
		AcceptsArgs: true,
		Args: []CommandArgDefinition{
			{Name: "model", Description: "Model id (provider/model or id)", Type: ArgTypeString},
		},
		ArgsParsing: ArgsParsingPositional,
		Scope:       CommandScopeBoth,
		Category:    CategoryOptions,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "models",
		NativeName:  "models",
		Description: "List model providers or provider models.",
		TextAliases: []string{"/models"},
		AcceptsArgs: true,
		ArgsParsing: ArgsParsingNone,
		Scope:       CommandScopeBoth,
		Category:    CategoryOptions,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "think",
		NativeName:  "think",
		Description: "Set thinking level.",
		TextAliases: []string{"/think", "/thinking", "/t"},
		AcceptsArgs: true,
		Args: []CommandArgDefinition{
			{Name: "level", Description: "off, minimal, low, medium, high, xhigh", Type: ArgTypeString,
				ChoicesProvider: func(ctx CommandArgChoiceContext) []CommandArgChoice {
					levels := ListThinkingLevels(ctx.Provider, ctx.Model)
					choices := make([]CommandArgChoice, len(levels))
					for i, l := range levels {
						s := string(l)
						choices[i] = CommandArgChoice{Value: s, Label: s}
					}
					return choices
				}},
		},
		ArgsParsing:  ArgsParsingPositional,
		ArgsMenuAuto: true,
		Scope:        CommandScopeBoth,
		Category:     CategoryOptions,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "verbose",
		NativeName:  "verbose",
		Description: "Toggle verbose mode.",
		TextAliases: []string{"/verbose", "/v"},
		AcceptsArgs: true,
		Args: []CommandArgDefinition{
			{Name: "mode", Description: "on or off", Type: ArgTypeString,
				Choices: []CommandArgChoice{
					{Value: "on", Label: "On"},
					{Value: "off", Label: "Off"},
				}},
		},
		ArgsParsing:  ArgsParsingPositional,
		ArgsMenuAuto: true,
		Scope:        CommandScopeBoth,
		Category:     CategoryOptions,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "reasoning",
		NativeName:  "reasoning",
		Description: "Toggle reasoning visibility.",
		TextAliases: []string{"/reasoning", "/reason"},
		AcceptsArgs: true,
		Args: []CommandArgDefinition{
			{Name: "mode", Description: "on, off, or stream", Type: ArgTypeString,
				Choices: []CommandArgChoice{
					{Value: "on", Label: "On"},
					{Value: "off", Label: "Off"},
					{Value: "stream", Label: "Stream"},
				}},
		},
		ArgsParsing:  ArgsParsingPositional,
		ArgsMenuAuto: true,
		Scope:        CommandScopeBoth,
		Category:     CategoryOptions,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "elevated",
		NativeName:  "elevated",
		Description: "Toggle elevated mode.",
		TextAliases: []string{"/elevated", "/elev"},
		AcceptsArgs: true,
		Args: []CommandArgDefinition{
			{Name: "mode", Description: "on, off, ask, or full", Type: ArgTypeString,
				Choices: []CommandArgChoice{
					{Value: "on", Label: "On"},
					{Value: "off", Label: "Off"},
					{Value: "ask", Label: "Ask"},
					{Value: "full", Label: "Full"},
				}},
		},
		ArgsParsing:  ArgsParsingPositional,
		ArgsMenuAuto: true,
		Scope:        CommandScopeBoth,
		Category:     CategoryOptions,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "exec",
		NativeName:  "exec",
		Description: "Set exec defaults for this session.",
		TextAliases: []string{"/exec"},
		AcceptsArgs: true,
		Args: []CommandArgDefinition{
			{Name: "options", Description: "host=... security=... ask=... node=...", Type: ArgTypeString},
		},
		ArgsParsing: ArgsParsingNone,
		Scope:       CommandScopeBoth,
		Category:    CategoryOptions,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "queue",
		NativeName:  "queue",
		Description: "Adjust queue settings.",
		TextAliases: []string{"/queue"},
		AcceptsArgs: true,
		Args: []CommandArgDefinition{
			{Name: "mode", Description: "queue mode", Type: ArgTypeString,
				Choices: []CommandArgChoice{
					{Value: "steer", Label: "Steer"},
					{Value: "interrupt", Label: "Interrupt"},
					{Value: "followup", Label: "Followup"},
					{Value: "collect", Label: "Collect"},
					{Value: "steer-backlog", Label: "Steer-Backlog"},
				}},
			{Name: "debounce", Description: "debounce duration (e.g. 500ms, 2s)", Type: ArgTypeString},
			{Name: "cap", Description: "queue cap", Type: ArgTypeNumber},
			{Name: "drop", Description: "drop policy", Type: ArgTypeString,
				Choices: []CommandArgChoice{
					{Value: "old", Label: "Old"},
					{Value: "new", Label: "New"},
					{Value: "summarize", Label: "Summarize"},
				}},
		},
		ArgsParsing: ArgsParsingNone,
		FormatArgs:  FormatQueueArgs,
		Scope:       CommandScopeBoth,
		Category:    CategoryOptions,
	})

	// --- 媒体类 (media) ---

	RegisterCommand(&ChatCommandDefinition{
		Key:         "tts",
		NativeName:  "tts",
		Description: "Control text-to-speech (TTS).",
		TextAliases: []string{"/tts"},
		AcceptsArgs: true,
		Args: []CommandArgDefinition{
			{Name: "action", Description: "TTS action", Type: ArgTypeString,
				Choices: []CommandArgChoice{
					{Value: "on", Label: "On"},
					{Value: "off", Label: "Off"},
					{Value: "status", Label: "Status"},
					{Value: "provider", Label: "Provider"},
					{Value: "limit", Label: "Limit"},
					{Value: "summary", Label: "Summary"},
					{Value: "audio", Label: "Audio"},
					{Value: "help", Label: "Help"},
				}},
			{Name: "value", Description: "Provider, limit, or text", Type: ArgTypeString, CaptureRemaining: true},
		},
		ArgsParsing: ArgsParsingPositional,
		ArgsMenu: &CommandArgMenuSpec{
			Arg: "action",
			Title: "TTS Actions:\n" +
				"• On – Enable TTS for responses\n" +
				"• Off – Disable TTS\n" +
				"• Status – Show current settings\n" +
				"• Provider – Set voice provider (edge, elevenlabs, openai)\n" +
				"• Limit – Set max characters for TTS\n" +
				"• Summary – Toggle AI summary for long texts\n" +
				"• Audio – Generate TTS from custom text\n" +
				"• Help – Show usage guide",
		},
		Scope:    CommandScopeBoth,
		Category: CategoryMedia,
	})

	// --- 工具类 (tools) ---

	RegisterCommand(&ChatCommandDefinition{
		Key:         "skill",
		NativeName:  "skill",
		Description: "Run a skill by name.",
		TextAliases: []string{"/skill"},
		AcceptsArgs: true,
		Args: []CommandArgDefinition{
			{Name: "name", Description: "Skill name", Type: ArgTypeString, Required: true},
			{Name: "input", Description: "Skill input", Type: ArgTypeString, CaptureRemaining: true},
		},
		ArgsParsing: ArgsParsingPositional,
		Scope:       CommandScopeBoth,
		Category:    CategoryTools,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "restart",
		NativeName:  "restart",
		Description: "Restart Crab Claw（蟹爪）.",
		TextAliases: []string{"/restart"},
		Scope:       CommandScopeBoth,
		Category:    CategoryTools,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "bash",
		Description: "Run host shell commands (host-only).",
		TextAliases: []string{"/bash"},
		AcceptsArgs: true,
		Args: []CommandArgDefinition{
			{Name: "command", Description: "Shell command", Type: ArgTypeString, CaptureRemaining: true},
		},
		ArgsParsing: ArgsParsingPositional,
		Scope:       CommandScopeText,
		Category:    CategoryTools,
	})

	// --- 管理类 (management) ---

	RegisterCommand(&ChatCommandDefinition{
		Key:         "allowlist",
		Description: "List/add/remove allowlist entries.",
		TextAliases: []string{"/allowlist"},
		AcceptsArgs: true,
		Scope:       CommandScopeText,
		Category:    CategoryManagement,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "approve",
		NativeName:  "approve",
		Description: "Approve or deny exec requests.",
		TextAliases: []string{"/approve"},
		AcceptsArgs: true,
		Scope:       CommandScopeBoth,
		Category:    CategoryManagement,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "subagents",
		NativeName:  "subagents",
		Description: "List/stop/log/info subagent runs for this session.",
		TextAliases: []string{"/subagents"},
		AcceptsArgs: true,
		Args: []CommandArgDefinition{
			{Name: "action", Description: "list | stop | log | info | send", Type: ArgTypeString,
				Choices: []CommandArgChoice{
					{Value: "list", Label: "list"},
					{Value: "stop", Label: "stop"},
					{Value: "log", Label: "log"},
					{Value: "info", Label: "info"},
					{Value: "send", Label: "send"},
				}},
			{Name: "target", Description: "Run id, index, or session key", Type: ArgTypeString},
			{Name: "value", Description: "Additional input (limit/message)", Type: ArgTypeString, CaptureRemaining: true},
		},
		ArgsParsing:  ArgsParsingPositional,
		ArgsMenuAuto: true,
		Scope:        CommandScopeBoth,
		Category:     CategoryManagement,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "config",
		NativeName:  "config",
		Description: "Show or set config values.",
		TextAliases: []string{"/config"},
		AcceptsArgs: true,
		Args: []CommandArgDefinition{
			{Name: "action", Description: "show | get | set | unset", Type: ArgTypeString,
				Choices: []CommandArgChoice{
					{Value: "show", Label: "show"},
					{Value: "get", Label: "get"},
					{Value: "set", Label: "set"},
					{Value: "unset", Label: "unset"},
				}},
			{Name: "path", Description: "Config path", Type: ArgTypeString},
			{Name: "value", Description: "Value for set", Type: ArgTypeString, CaptureRemaining: true},
		},
		ArgsParsing: ArgsParsingNone,
		FormatArgs:  FormatConfigArgs,
		Scope:       CommandScopeBoth,
		Category:    CategoryManagement,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "debug",
		NativeName:  "debug",
		Description: "Set runtime debug overrides.",
		TextAliases: []string{"/debug"},
		AcceptsArgs: true,
		Args: []CommandArgDefinition{
			{Name: "action", Description: "show | reset | set | unset", Type: ArgTypeString,
				Choices: []CommandArgChoice{
					{Value: "show", Label: "show"},
					{Value: "reset", Label: "reset"},
					{Value: "set", Label: "set"},
					{Value: "unset", Label: "unset"},
				}},
			{Name: "path", Description: "Debug path", Type: ArgTypeString},
			{Name: "value", Description: "Value for set", Type: ArgTypeString, CaptureRemaining: true},
		},
		ArgsParsing: ArgsParsingNone,
		FormatArgs:  FormatDebugArgs,
		Scope:       CommandScopeBoth,
		Category:    CategoryManagement,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "activation",
		NativeName:  "activation",
		Description: "Set group activation mode.",
		TextAliases: []string{"/activation"},
		AcceptsArgs: true,
		Args: []CommandArgDefinition{
			{Name: "mode", Description: "mention or always", Type: ArgTypeString,
				Choices: []CommandArgChoice{
					{Value: "mention", Label: "Mention"},
					{Value: "always", Label: "Always"},
				}},
		},
		ArgsParsing:  ArgsParsingPositional,
		ArgsMenuAuto: true,
		Scope:        CommandScopeBoth,
		Category:     CategoryManagement,
	})

	RegisterCommand(&ChatCommandDefinition{
		Key:         "send",
		NativeName:  "send",
		Description: "Set send policy.",
		TextAliases: []string{"/send"},
		AcceptsArgs: true,
		Args: []CommandArgDefinition{
			{Name: "mode", Description: "on, off, or inherit", Type: ArgTypeString,
				Choices: []CommandArgChoice{
					{Value: "on", Label: "On"},
					{Value: "off", Label: "Off"},
					{Value: "inherit", Label: "Inherit"},
				}},
		},
		ArgsParsing:  ArgsParsingPositional,
		ArgsMenuAuto: true,
		Scope:        CommandScopeBoth,
		Category:     CategoryManagement,
	})
}
