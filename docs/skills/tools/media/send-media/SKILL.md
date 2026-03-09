---
name: send-media
description: "Send files and media to remote channels (Feishu/Discord/Telegram/WhatsApp) with 30MB limit"
tools: send_media
metadata:
  tree_id: "media/send_media"
  tree_group: "media"
  min_tier: "task_light"
  approval_type: "data_export"
---

# Send Media — Remote File Delivery

## Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `file_path` | preferred | Absolute path within tool-accessible range |
| `media_base64` | alt | Base64 data (only when file data is in memory) |
| `file_name` | no | Override filename (auto-inferred from path basename) |
| `target` | no | `channel:id` format; omitted = current session route |
| `mime_type` | no | Auto-detected from extension |
| `message` | no | Accompanying text message |

## Constraints

- **30 MB hard limit** — compress/split or use alternative delivery for larger files
- Requires **data_export** approval (mount access for file reading)
- Auto-uses basename as remote filename
- MIME auto-detection for common types (images, PDF, Office)

## Troubleshooting

| Error | Cause | Fix |
|-------|-------|-----|
| "Media sender not available" | No channel sender in context | Verify channel configuration |
| "No target and no session channel" | Missing route + no explicit target | Specify `target` explicitly |
| Path access errors | Permission scope issue | Verify absolute path + mount |
| 30MB exceeded | File too large | Compress, split, or alt delivery |
