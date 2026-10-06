/// Url and bearer token for the loopback API. `buti desktop` puts both in the
/// environment of this process; they are not compiled into the frontend.
#[derive(serde::Serialize)]
struct ApiConfig {
  url: String,
  token: String,
}

#[tauri::command]
fn api_config() -> ApiConfig {
  ApiConfig {
    url: std::env::var("BUTI_API_URL").unwrap_or_default(),
    token: std::env::var("BUTI_API_TOKEN").unwrap_or_default(),
  }
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
  tauri::Builder::default()
    // Opens pull request links in the system browser.
    .plugin(tauri_plugin_opener::init())
    // The folder picker: unlike the TUI, the window is not started in a repository.
    .plugin(tauri_plugin_dialog::init())
    .invoke_handler(tauri::generate_handler![api_config])
    .setup(|app| {
      if cfg!(debug_assertions) {
        app.handle().plugin(
          tauri_plugin_log::Builder::default()
            .level(log::LevelFilter::Info)
            .build(),
        )?;
      }
      Ok(())
    })
    .run(tauri::generate_context!())
    .expect("error while building tauri application");
}
