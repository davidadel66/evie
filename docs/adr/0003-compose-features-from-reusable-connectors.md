# Compose feature plugins from reusable connector plugins

Business-specific behavior belongs in a Feature Plugin that composes reusable
Connector Plugins instead of embedding their external-system access. Cairo's
Kitchen will therefore own its restaurant language, rules, and procedures while
depending on separate Square, Google Sheets, and eventual payout plugins. This
keeps Cairo's business logic cohesive while allowing the same connectors to
support unrelated features without inheriting restaurant behavior.
