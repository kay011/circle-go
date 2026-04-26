---
name: hello-world
description: 最小 hello world skill，用于验证 skill 路由与前端展示链路。
tools:
  - name: hello_world
    id: skill.hello_world
    version: "1.0.0"
    display_name: Hello World Skill
    owner: skills/hello-world
    risk_level: low
    intent_tags: ["hello", "问候", "示例", "skill测试"]
    policy:
      timeout_seconds: 8
      approval: never
---

# Hello World Skill

## 使用场景

- 用户提到 `hello`、`hello world`、`打个招呼`
- 需要验证 skill 触发与前端状态展示

## 行为约束

- 优先调用 `hello_world` 工具
- 输出简洁问候，不展开复杂推理

