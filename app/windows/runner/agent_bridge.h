#pragma once
#include <flutter/method_channel.h>
#include <flutter/standard_method_codec.h>
#include <windows.h>
#include <atomic>
#include <thread>

class AgentBridge {
 public:
  static constexpr UINT kComplete = WM_APP + 42;
  AgentBridge(flutter::BinaryMessenger* messenger, HWND window);
  ~AgentBridge();
  void Complete();
 private:
  HWND window_;
  std::unique_ptr<flutter::MethodChannel<flutter::EncodableValue>> channel_;
  std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>> result_;
  std::thread worker_;
  std::atomic<bool> busy_{false};
  std::string value_, error_;
};
