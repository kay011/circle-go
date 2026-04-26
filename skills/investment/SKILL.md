---
name: investment
description: 投资分析与基金对比技能。适用于股票/基金分析、估值、基金夏普与回撤对比等场景。
tools:
  - name: investment_analyzer
    id: skill.investment.analyzer
    version: "1.0.0"
    display_name: 投资分析 Skill
    owner: skills/investment
    risk_level: medium
    intent_tags: ["股票分析", "基金分析", "估值", "量化"]
    policy:
      timeout_seconds: 22
      approval: never
  - name: fund_compare
    id: skill.investment.fund_compare
    version: "1.0.0"
    display_name: 基金对比 Skill
    owner: skills/investment
    risk_level: medium
    intent_tags: ["基金对比", "夏普", "回撤", "收益率", "规模"]
    policy:
      timeout_seconds: 28
      approval: never
---

# Investment Skill

## 使用场景

- 用户要求分析股票或基金投资价值
- 用户要求对比多个基金的收益、夏普、回撤、规模

## 路由建议

- 单标的分析优先 `investment_analyzer`
- 多基金对比优先 `fund_compare`

## 输出规范

- 缺失字段显示为 `N/A`
- 量化指标后附简短解释，不夸大结论
- 明确免责声明，不构成投资建议

