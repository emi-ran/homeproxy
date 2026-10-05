#include "agent_bridge.h"
#include <sddl.h>
#include <shellapi.h>
#include <winsvc.h>
#include <vector>

namespace {
struct Handle {
  HANDLE value;
  explicit Handle(HANDLE h) : value(h) {}
  ~Handle() { if (value && value != INVALID_HANDLE_VALUE) CloseHandle(value); }
};
struct ServiceHandle {
  SC_HANDLE value;
  explicit ServiceHandle(SC_HANDLE h) : value(h) {}
  ~ServiceHandle() { if (value) CloseServiceHandle(value); }
};
std::string WinError(const char* action, DWORD code = GetLastError()) {
  return std::string(action) + " (Windows error " + std::to_string(code) +
      "). Check service state and authorized Windows user.";
}
bool ServiceStatus(SERVICE_STATUS_PROCESS& status, std::string& error) {
  ServiceHandle manager(OpenSCManagerW(nullptr, nullptr, SC_MANAGER_CONNECT));
  if (!manager.value) { error = WinError("Cannot query SCM"); return false; }
  ServiceHandle service(OpenServiceW(manager.value, L"HomeProxyAgent", SERVICE_QUERY_STATUS));
  if (!service.value) {
    error = GetLastError() == ERROR_SERVICE_DOES_NOT_EXIST ? "missing" : WinError("Cannot query service");
    return false;
  }
  DWORD bytes = 0;
  if (!QueryServiceStatusEx(service.value, SC_STATUS_PROCESS_INFO,
      reinterpret_cast<BYTE*>(&status), sizeof(status), &bytes)) {
    error = WinError("Cannot read service status"); return false;
  }
  return true;
}
bool Transfer(HANDLE pipe, bool write, char* data, DWORD size, DWORD& count,
              ULONGLONG deadline, std::string& error) {
  Handle event(CreateEventW(nullptr, TRUE, FALSE, nullptr));
  if (!event.value) { error = WinError("Cannot create I/O event"); return false; }
  OVERLAPPED operation{};
  operation.hEvent = event.value;
  BOOL ok = write ? WriteFile(pipe, data, size, &count, &operation)
                  : ReadFile(pipe, data, size, &count, &operation);
  if (!ok && GetLastError() != ERROR_IO_PENDING) {
    error = WinError("Pipe I/O failed"); return false;
  }
  if (!ok) {
    auto now = GetTickCount64();
    DWORD wait = WaitForSingleObject(event.value, now < deadline ? static_cast<DWORD>(deadline - now) : 0);
    if (wait != WAIT_OBJECT_0) {
      CancelIoEx(pipe, &operation);
      // Drain cancellation before stack OVERLAPPED/event are destroyed.
      GetOverlappedResult(pipe, &operation, &count, TRUE);
      error = "Pipe I/O timed out. Check service, then retry."; return false;
    }
    if (!GetOverlappedResult(pipe, &operation, &count, FALSE)) {
      error = WinError("Pipe I/O failed"); return false;
    }
  }
  return true;
}
std::string PipeRequest(std::string request, std::string& error) {
  if (request.empty() || request.size() + 1 > 16384 || request.find('\n') != std::string::npos) {
    error = "Invalid or oversized IPC request."; return {};
  }
  constexpr auto path = L"\\\\.\\pipe\\HomeProxyAgent.v1";
  HANDLE raw = INVALID_HANDLE_VALUE;
  for (int attempt = 0; attempt < 4; ++attempt) {
    // Specific rights exclude FILE_CREATE_PIPE_INSTANCE. Never use GENERIC_WRITE.
    raw = CreateFileW(path, 0x12019b, 0, nullptr, OPEN_EXISTING,
                      FILE_FLAG_OVERLAPPED | SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION, nullptr);
    if (raw != INVALID_HANDLE_VALUE) break;
    DWORD code = GetLastError();
    if (code != ERROR_PIPE_BUSY) {
      error = code == ERROR_ACCESS_DENIED ? "Pipe access denied. Sign in as authorized installation user."
          : code == ERROR_FILE_NOT_FOUND ? "Service pipe unavailable. Install/start service, then retry."
          : WinError("Cannot open service pipe", code);
      return {};
    }
    WaitNamedPipeW(path, 250);
  }
  if (raw == INVALID_HANDLE_VALUE) { error = "Service pipe busy. Retry shortly."; return {}; }
  Handle pipe(raw);
  ULONG pid = 0;
  SERVICE_STATUS_PROCESS status{};
  // Verify on every connection BEFORE any bytes (including token) are written.
  if (!GetNamedPipeServerProcessId(pipe.value, &pid) || !ServiceStatus(status, error) ||
      status.dwCurrentState != SERVICE_RUNNING || !pid || pid != status.dwProcessId) {
    error = "Service pipe identity check failed. No settings sent. Check HomeProxyAgent SCM state.";
    return {};
  }
  request += '\n';
  const ULONGLONG deadline = GetTickCount64() + 4500;
  DWORD offset = 0;
  while (offset < request.size()) {
    DWORD count = 0;
    if (!Transfer(pipe.value, true, request.data() + offset,
                  static_cast<DWORD>(request.size()) - offset, count, deadline, error)) return {};
    if (!count) { error = "Pipe closed during request."; return {}; }
    offset += count;
  }
  std::string response;
  while (response.size() < 16384) {
    char buffer[1024];
    DWORD count = 0;
    if (!Transfer(pipe.value, false, buffer, sizeof(buffer), count, deadline, error)) return {};
    if (!count) { error = "Service returned incomplete response."; return {}; }
    response.append(buffer, count);
    if (response.size() > 16384) { error = "Service response exceeds IPC limit."; return {}; }
    auto newline = response.find('\n');
    if (newline != std::string::npos) return response.substr(0, newline);
  }
  error = "Service response exceeds IPC limit."; return {};
}
std::wstring UserSID(std::string& error) {
  HANDLE raw = nullptr;
  if (!OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &raw)) {
    error = WinError("Cannot capture original user SID"); return {};
  }
  Handle token(raw);
  DWORD size = 0;
  GetTokenInformation(token.value, TokenUser, nullptr, 0, &size);
  std::vector<BYTE> buffer(size);
  if (!size || !GetTokenInformation(token.value, TokenUser, buffer.data(), size, &size)) {
    error = WinError("Cannot read original user SID"); return {};
  }
  LPWSTR sid = nullptr;
  if (!ConvertSidToStringSidW(reinterpret_cast<TOKEN_USER*>(buffer.data())->User.Sid, &sid)) {
    error = WinError("Cannot encode user SID"); return {};
  }
  std::wstring value(sid);
  LocalFree(sid);
  return value;
}
std::string ServiceCommand(const std::string& action, HWND window, std::string& error) {
  struct COMScope {
    HRESULT hr = CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);
    ~COMScope() { if (SUCCEEDED(hr)) CoUninitialize(); }
  } com;
  if (FAILED(com.hr)) { error = "Cannot initialize elevation bridge."; return {}; }
  if (action != "install" && action != "uninstall" && action != "start" && action != "stop") {
    error = "Unsupported SCM action."; return {};
  }
  std::wstring args = L"service " + std::wstring(action.begin(), action.end());
  if (action == "install") {
    auto sid = UserSID(error); // Original GUI process token, not elevated helper token.
    if (!error.empty()) return {};
    args += L" --user-sid " + sid;
  }
  wchar_t module[32768];
  DWORD length = GetModuleFileNameW(nullptr, module, 32768);
  if (!length || length >= 32768) { error = "Cannot locate packaged Go helper."; return {}; }
  std::wstring file(module, length);
  file = file.substr(0, file.find_last_of(L"\\/") + 1) + L"homeproxy.exe";
  if (GetFileAttributesW(file.c_str()) == INVALID_FILE_ATTRIBUTES) {
    error = "Packaged homeproxy.exe missing. Copy Go helper beside homeproxy-gui.exe."; return {};
  }
  SHELLEXECUTEINFOW info{};
  info.cbSize = sizeof(info);
  info.fMask = SEE_MASK_NOCLOSEPROCESS | SEE_MASK_NOASYNC | SEE_MASK_FLAG_NO_UI;
  info.hwnd = window;
  info.lpVerb = L"runas";
  info.lpFile = file.c_str();
  info.lpParameters = args.c_str();
  info.nShow = SW_SHOWNORMAL;
  if (!ShellExecuteExW(&info)) {
    DWORD code = GetLastError();
    error = code == ERROR_CANCELLED ? "UAC cancelled. Service unchanged. Approve UAC to continue."
                                   : WinError("Cannot launch elevated Go helper", code);
    return {};
  }
  Handle process(info.hProcess);
  if (!process.value) { error = "Helper process handle missing; check SCM state."; return {}; }
  if (WaitForSingleObject(process.value, 120000) != WAIT_OBJECT_0) {
    error = "Helper wait timed out. It may still finish; inspect SCM before retrying. Helper was not killed.";
    return {};
  }
  DWORD code = 0;
  if (!GetExitCodeProcess(process.value, &code)) { error = WinError("Cannot read helper exit"); return {}; }
  if (code) {
    error = "Go helper exited with code " + std::to_string(code) +
      ". Inspect elevated PowerShell: homeproxy.exe service status. Installation may need repair; uninstall requires Stopped state.";
    return {};
  }
  return "ok";
}
} // namespace

AgentBridge::AgentBridge(flutter::BinaryMessenger* messenger, HWND window) : window_(window) {
  channel_ = std::make_unique<flutter::MethodChannel<flutter::EncodableValue>>(
      messenger, "homeproxy/windows", &flutter::StandardMethodCodec::GetInstance());
  channel_->SetMethodCallHandler([this](const auto& call, auto result) {
    const auto method = call.method_name();
    if (method != "serviceStatus" && method != "request" && method != "serviceCommand") {
      result->NotImplemented(); return;
    }
    if (busy_) { result->Error("busy", "Native operation in progress. Retry shortly."); return; }
    const auto* argument = call.arguments() ? std::get_if<std::string>(call.arguments()) : nullptr;
    if (method != "serviceStatus" && !argument) {
      result->Error("argument", "Expected string argument."); return;
    }
    if (worker_.joinable()) worker_.join();
    busy_ = true;
    result_ = std::move(result);
    std::string input = argument ? *argument : "";
    worker_ = std::thread([this, method, input]() {
      error_.clear();
      value_.clear();
      if (method == "request") value_ = PipeRequest(input, error_);
      else if (method == "serviceCommand") value_ = ServiceCommand(input, window_, error_);
      else {
        SERVICE_STATUS_PROCESS status{};
        if (ServiceStatus(status, error_)) {
          value_ = status.dwCurrentState == SERVICE_RUNNING ? "running"
                 : status.dwCurrentState == SERVICE_STOPPED ? "stopped" : "pending";
        } else if (error_ == "missing") { value_ = "missing"; error_.clear(); }
      }
      PostMessageW(window_, kComplete, 0, 0);
    });
  });
}
AgentBridge::~AgentBridge() {
  channel_->SetMethodCallHandler(nullptr);
  if (worker_.joinable()) worker_.join();
}
void AgentBridge::Complete() {
  if (!busy_) return;
  worker_.join();
  if (error_.empty()) result_->Success(flutter::EncodableValue(value_));
  else result_->Error("windows_agent", error_);
  result_.reset();
  busy_ = false;
}
