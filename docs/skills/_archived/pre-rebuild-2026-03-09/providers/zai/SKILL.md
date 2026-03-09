---
name: zai
description: "Use Z.AI (GLM models) with Crab Claw（蟹爪）"
---

# Z.AI

Z.AI is the API platform for **GLM** models. It provides REST APIs for GLM and uses API keys
for authentication. Create your API key in the Z.AI console. Crab Claw（蟹爪） uses the `zai` provider
with a Z.AI API key.

## CLI setup

```bash
crabclaw onboard --auth-choice zai-api-key
# or non-interactive
crabclaw onboard --zai-api-key "$ZAI_API_KEY"
```

## Config snippet

```json5
{
  env: { ZAI_API_KEY: "sk-..." },
  agents: { defaults: { model: { primary: "zai/glm-4.7" } } },
}
```

## Notes

- GLM models are available as `zai/<model>` (example: `zai/glm-4.7`).
- See [/providers/glm](/providers/glm) for the model family overview.
- Z.AI uses Bearer auth with your API key.
