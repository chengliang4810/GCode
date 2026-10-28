fn main() {
    let registry = zeron_theme_reference_export::builtin_registry();
    println!("{}", serde_json::to_string_pretty(registry).unwrap());
}
