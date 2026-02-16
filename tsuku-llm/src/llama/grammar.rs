//! JSON Schema to GBNF grammar conversion.
//!
//! This module provides conversion from JSON Schema (as used by tsuku tools)
//! to GBNF (GGML BNF) grammar strings that constrain llama.cpp token generation.

use std::collections::{HashMap, HashSet};
use std::ffi::CString;
use std::ptr::NonNull;

use super::bindings::{
    llama_sampler, llama_sampler_accept, llama_sampler_chain_add, llama_sampler_chain_default_params,
    llama_sampler_chain_init, llama_sampler_free, llama_sampler_init_grammar,
    llama_sampler_init_greedy, llama_sampler_sample, llama_vocab,
};
use super::error::{LlamaError, Result};

/// A sampler chain that includes a grammar constraint.
///
/// This wraps the llama.cpp sampler chain API and ensures proper cleanup.
pub struct GrammarSampler {
    chain: NonNull<llama_sampler>,
}

// SAFETY: GrammarSampler owns the sampler chain and ensures single-threaded access.
unsafe impl Send for GrammarSampler {}

impl GrammarSampler {
    /// Create a new grammar-constrained sampler.
    ///
    /// # Arguments
    ///
    /// * `vocab` - The vocabulary from the model
    /// * `grammar_str` - GBNF grammar string
    /// * `grammar_root` - Name of the root rule (typically "root")
    ///
    /// # Returns
    ///
    /// A sampler chain with grammar constraint followed by greedy sampling.
    /// Returns an error if the grammar is invalid.
    pub fn new(
        vocab: *const llama_vocab,
        grammar_str: &str,
        grammar_root: &str,
    ) -> Result<Self> {
        let grammar_c = CString::new(grammar_str).map_err(|e| {
            LlamaError::Grammar(format!("Invalid grammar string: {}", e))
        })?;
        let root_c = CString::new(grammar_root).map_err(|e| {
            LlamaError::Grammar(format!("Invalid root rule name: {}", e))
        })?;

        unsafe {
            // Create the sampler chain
            let params = llama_sampler_chain_default_params();
            let chain = llama_sampler_chain_init(params);
            if chain.is_null() {
                return Err(LlamaError::Grammar("Failed to create sampler chain".to_string()));
            }

            // Create grammar sampler
            let grammar = llama_sampler_init_grammar(
                vocab,
                grammar_c.as_ptr(),
                root_c.as_ptr(),
            );
            if grammar.is_null() {
                llama_sampler_free(chain);
                return Err(LlamaError::Grammar(format!(
                    "Failed to create grammar sampler. Grammar may be invalid:\n{}",
                    grammar_str
                )));
            }

            // Add grammar to chain
            llama_sampler_chain_add(chain, grammar);

            // Add greedy sampler (temperature 0)
            let greedy = llama_sampler_init_greedy();
            llama_sampler_chain_add(chain, greedy);

            Ok(Self {
                chain: NonNull::new_unchecked(chain),
            })
        }
    }

    /// Sample the next token using the grammar-constrained sampler chain.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The llama context
    /// * `idx` - The batch index to sample from
    ///
    /// # Returns
    ///
    /// The sampled token ID.
    pub fn sample(&mut self, ctx: *mut super::bindings::llama_context, idx: i32) -> i32 {
        unsafe { llama_sampler_sample(self.chain.as_ptr(), ctx, idx) }
    }

    /// Accept a token to update the grammar state.
    ///
    /// This must be called after each sampled token to advance the grammar.
    pub fn accept(&mut self, token: i32) {
        unsafe {
            llama_sampler_accept(self.chain.as_ptr(), token);
        }
    }
}

impl Drop for GrammarSampler {
    fn drop(&mut self) {
        unsafe {
            llama_sampler_free(self.chain.as_ptr());
        }
    }
}

/// Generate a GBNF grammar string from a JSON Schema.
///
/// This supports the subset of JSON Schema used by tsuku tools:
/// - Object types with properties
/// - Array types with item schemas
/// - Primitive types: string, number, boolean, null
/// - Required vs optional properties
/// - Nested objects and arrays
///
/// # Arguments
///
/// * `schema` - A JSON Schema as a serde_json Value
///
/// # Returns
///
/// A GBNF grammar string with "root" as the start symbol.
pub fn json_schema_to_gbnf(schema: &serde_json::Value) -> Result<String> {
    let mut builder = GbnfBuilder::new();
    builder.process_schema(schema, "root")?;
    Ok(builder.build())
}

/// Builder for constructing GBNF grammar strings.
struct GbnfBuilder {
    rules: Vec<String>,
    defined_rules: HashSet<String>,
}

impl GbnfBuilder {
    fn new() -> Self {
        Self {
            rules: Vec::new(),
            defined_rules: HashSet::new(),
        }
    }

    /// Process a JSON Schema and generate rules for the given rule name.
    fn process_schema(&mut self, schema: &serde_json::Value, rule_name: &str) -> Result<()> {
        let obj = schema.as_object().ok_or_else(|| {
            LlamaError::Grammar("Schema must be an object".to_string())
        })?;

        let schema_type = obj.get("type").and_then(|v| v.as_str());

        match schema_type {
            Some("object") => self.process_object(schema, rule_name)?,
            Some("array") => self.process_array(schema, rule_name)?,
            Some("string") => self.add_string_rule(rule_name),
            Some("number") | Some("integer") => self.add_number_rule(rule_name),
            Some("boolean") => self.add_boolean_rule(rule_name),
            Some("null") => self.add_null_rule(rule_name),
            None => {
                // No type specified, allow any value
                self.add_any_value_rule(rule_name);
            }
            Some(t) => {
                return Err(LlamaError::Grammar(format!("Unknown type: {}", t)));
            }
        }

        Ok(())
    }

    /// Process an object schema.
    fn process_object(&mut self, schema: &serde_json::Value, rule_name: &str) -> Result<()> {
        let obj = schema.as_object().unwrap();
        let properties = obj.get("properties").and_then(|v| v.as_object());
        let required: HashSet<&str> = obj
            .get("required")
            .and_then(|v| v.as_array())
            .map(|arr| arr.iter().filter_map(|v| v.as_str()).collect())
            .unwrap_or_default();

        if properties.is_none() || properties.unwrap().is_empty() {
            // Empty object or no properties defined
            self.add_rule(rule_name, r#""{" ws "}""#);
            return Ok(());
        }

        let properties = properties.unwrap();

        // Separate required and optional properties
        let mut required_props: Vec<(&str, &serde_json::Value)> = Vec::new();
        let mut optional_props: Vec<(&str, &serde_json::Value)> = Vec::new();

        for (name, prop_schema) in properties {
            if required.contains(name.as_str()) {
                required_props.push((name.as_str(), prop_schema));
            } else {
                optional_props.push((name.as_str(), prop_schema));
            }
        }

        // Generate key-value rules for each property
        let mut kv_rules: HashMap<&str, String> = HashMap::new();
        for (name, prop_schema) in properties {
            let kv_rule_name = format!("{}-{}-kv", rule_name, sanitize_name(name));
            let value_rule_name = format!("{}-{}", rule_name, sanitize_name(name));

            // Process the property schema
            self.process_schema(prop_schema, &value_rule_name)?;

            // Create key-value rule
            let kv_rule = format!(
                r#""\"{}\"" ws ":" ws {}"#,
                escape_json_key(name),
                value_rule_name
            );
            self.add_rule(&kv_rule_name, &kv_rule);
            kv_rules.insert(name.as_str(), kv_rule_name);
        }

        // Build the object rule
        let mut object_rule = String::from(r#""{" ws "#);

        // Add required properties
        for (i, (name, _)) in required_props.iter().enumerate() {
            if i > 0 {
                object_rule.push_str(r#" "," ws "#);
            }
            object_rule.push_str(&kv_rules[name]);
        }

        // Add optional properties (each wrapped in optional marker)
        for (name, _) in &optional_props {
            if !required_props.is_empty() || optional_props.iter().position(|(n, _)| n == name).unwrap() > 0 {
                object_rule.push_str(&format!(r#" ("," ws {})?"#, &kv_rules[name]));
            } else {
                object_rule.push_str(&format!("({})?", &kv_rules[name]));
            }
        }

        object_rule.push_str(r#" ws "}""#);

        self.add_rule(rule_name, &object_rule);

        Ok(())
    }

    /// Process an array schema.
    fn process_array(&mut self, schema: &serde_json::Value, rule_name: &str) -> Result<()> {
        let obj = schema.as_object().unwrap();

        // Get items schema
        if let Some(items) = obj.get("items") {
            let item_rule_name = format!("{}-item", rule_name);
            self.process_schema(items, &item_rule_name)?;

            // Array with typed items
            let array_rule = format!(
                r#""[" ws ({} ("," ws {})*)? ws "]""#,
                item_rule_name, item_rule_name
            );
            self.add_rule(rule_name, &array_rule);
        } else {
            // Array with any items
            self.add_rule(rule_name, r#""[" ws (value ("," ws value)*)? ws "]""#);
        }

        Ok(())
    }

    fn add_string_rule(&mut self, rule_name: &str) {
        if !self.defined_rules.contains(rule_name) {
            // Use the same string definition as json.gbnf
            self.add_rule(
                rule_name,
                r#""\"" ([^"\\\x7F\x00-\x1F] | "\\" (["\\bfnrt] | "u" [0-9a-fA-F]{4}))* "\"" ws"#,
            );
        }
    }

    fn add_number_rule(&mut self, rule_name: &str) {
        if !self.defined_rules.contains(rule_name) {
            self.add_rule(
                rule_name,
                r#"("-"? ([0-9] | [1-9] [0-9]{0,15})) ("." [0-9]+)? ([eE] [-+]? [0-9] [1-9]{0,15})? ws"#,
            );
        }
    }

    fn add_boolean_rule(&mut self, rule_name: &str) {
        if !self.defined_rules.contains(rule_name) {
            self.add_rule(rule_name, r#"("true" | "false") ws"#);
        }
    }

    fn add_null_rule(&mut self, rule_name: &str) {
        if !self.defined_rules.contains(rule_name) {
            self.add_rule(rule_name, r#""null" ws"#);
        }
    }

    fn add_any_value_rule(&mut self, rule_name: &str) {
        if !self.defined_rules.contains(rule_name) {
            // Ensure base rules exist
            self.ensure_base_rules();
            self.add_rule(rule_name, "value");
        }
    }

    fn ensure_base_rules(&mut self) {
        if !self.defined_rules.contains("value") {
            self.add_rule(
                "value",
                r#"object | array | string | number | ("true" | "false" | "null") ws"#,
            );
        }
        if !self.defined_rules.contains("object") {
            self.add_rule(
                "object",
                r#""{" ws (string ":" ws value ("," ws string ":" ws value)*)? ws "}""#,
            );
        }
        if !self.defined_rules.contains("array") {
            self.add_rule(
                "array",
                r#""[" ws (value ("," ws value)*)? ws "]""#,
            );
        }
        if !self.defined_rules.contains("string") {
            self.add_rule(
                "string",
                r#""\"" ([^"\\\x7F\x00-\x1F] | "\\" (["\\bfnrt] | "u" [0-9a-fA-F]{4}))* "\"" ws"#,
            );
        }
        if !self.defined_rules.contains("number") {
            self.add_rule(
                "number",
                r#"("-"? ([0-9] | [1-9] [0-9]{0,15})) ("." [0-9]+)? ([eE] [-+]? [0-9] [1-9]{0,15})? ws"#,
            );
        }
    }

    fn add_rule(&mut self, name: &str, definition: &str) {
        if !self.defined_rules.contains(name) {
            self.rules.push(format!("{} ::= {}", name, definition));
            self.defined_rules.insert(name.to_string());
        }
    }

    fn build(mut self) -> String {
        // Add whitespace rule at the end
        if !self.defined_rules.contains("ws") {
            self.rules.push(r#"ws ::= | " " | "\n" [ \t]{0,20}"#.to_string());
        }

        self.rules.join("\n")
    }
}

/// Sanitize a property name for use in a rule name.
fn sanitize_name(name: &str) -> String {
    name.chars()
        .map(|c| if c.is_alphanumeric() { c } else { '-' })
        .collect()
}

/// Escape special characters in a JSON key for GBNF.
fn escape_json_key(key: &str) -> String {
    key.replace('\\', "\\\\").replace('"', "\\\"")
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn test_simple_object_schema() {
        let schema = json!({
            "type": "object",
            "properties": {
                "path": { "type": "string" }
            },
            "required": ["path"]
        });

        let grammar = json_schema_to_gbnf(&schema).unwrap();
        eprintln!("Grammar:\n{}", grammar);

        // Should contain root rule and path-kv rule
        assert!(grammar.contains("root ::="), "root rule missing");
        assert!(grammar.contains("root-path-kv"), "path-kv rule missing");
        // The key should be escaped with backslash-quote in GBNF
        assert!(grammar.contains(r#"\"path\""#), "path key missing in grammar: {}", grammar);
    }

    #[test]
    fn test_object_with_optional_properties() {
        let schema = json!({
            "type": "object",
            "properties": {
                "name": { "type": "string" },
                "age": { "type": "number" }
            },
            "required": ["name"]
        });

        let grammar = json_schema_to_gbnf(&schema).unwrap();

        // age should be optional
        assert!(grammar.contains("root-age-kv)?"));
    }

    #[test]
    fn test_array_schema() {
        let schema = json!({
            "type": "array",
            "items": {
                "type": "string"
            }
        });

        let grammar = json_schema_to_gbnf(&schema).unwrap();

        assert!(grammar.contains("root ::="));
        assert!(grammar.contains("["));
        assert!(grammar.contains("root-item"));
    }

    #[test]
    fn test_nested_object_schema() {
        let schema = json!({
            "type": "object",
            "properties": {
                "mappings": {
                    "type": "array",
                    "items": {
                        "type": "object",
                        "properties": {
                            "asset": { "type": "string" },
                            "os": { "type": "string" }
                        },
                        "required": ["asset", "os"]
                    }
                }
            },
            "required": ["mappings"]
        });

        let grammar = json_schema_to_gbnf(&schema).unwrap();

        // Should have nested rules
        assert!(grammar.contains("root-mappings"));
        assert!(grammar.contains("root-mappings-item"));
        assert!(grammar.contains("asset"));
        assert!(grammar.contains("os"));
    }

    #[test]
    fn test_fetch_file_schema() {
        // This is the actual fetch_file tool schema from tsuku
        let schema = json!({
            "type": "object",
            "properties": {
                "path": {
                    "type": "string",
                    "description": "File path in repo"
                }
            },
            "required": ["path"]
        });

        let grammar = json_schema_to_gbnf(&schema).unwrap();

        assert!(grammar.contains("root ::="), "root rule missing");
        // The key should be escaped with backslash-quote in GBNF
        assert!(grammar.contains(r#"\"path\""#), "path key missing: {}", grammar);
        // Root should require path
        assert!(grammar.contains("root-path-kv"), "path-kv rule missing");
    }

    #[test]
    fn test_extract_pattern_schema() {
        // This is the actual extract_pattern tool schema from tsuku
        let schema = json!({
            "type": "object",
            "properties": {
                "mappings": {
                    "type": "array",
                    "items": {
                        "type": "object",
                        "properties": {
                            "asset": { "type": "string" },
                            "os": { "type": "string" },
                            "arch": { "type": "string" },
                            "format": { "type": "string" }
                        },
                        "required": ["asset", "os", "arch", "format"]
                    }
                },
                "executable": { "type": "string" },
                "verify_command": { "type": "string" },
                "strip_prefix": { "type": "string" },
                "install_subpath": { "type": "string" }
            },
            "required": ["mappings", "executable", "verify_command"]
        });

        let grammar = json_schema_to_gbnf(&schema).unwrap();

        // Required fields should be in root
        assert!(grammar.contains("root-mappings-kv"));
        assert!(grammar.contains("root-executable-kv"));
        assert!(grammar.contains("root-verify-command-kv"));

        // Optional fields should be marked optional
        assert!(grammar.contains("root-strip-prefix-kv)?"));
        assert!(grammar.contains("root-install-subpath-kv)?"));

        // Nested mapping properties
        assert!(grammar.contains("root-mappings-item"));
    }
}
