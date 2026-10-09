import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

import '../app/lib/windows_agent.dart';

void main() {
  testWidgets('unchanged Windows status preserves widget identity', (
    tester,
  ) async {
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      const MethodChannel('homeproxy/windows'),
      (call) async {
        if (call.method == 'serviceStatus') return 'running';
        return jsonEncode({'version': 1, 'ok': true, 'status': 'Durduruldu'});
      },
    );
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pumpWidget(const MaterialApp(home: WindowsAgentScreen()));
    await tester.pump();
    final before = tester.widget<Text>(find.text('Tünel: Durduruldu'));
    await tester.pump(const Duration(seconds: 2));
    await tester.pump();
    expect(
      identical(before, tester.widget<Text>(find.text('Tünel: Durduruldu'))),
      isTrue,
    );
    await tester.pumpWidget(const SizedBox());
  });
  testWidgets('Windows polling pauses offscreen and refreshes on resume', (
    tester,
  ) async {
    var scm = 0, pipe = 0;
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      const MethodChannel('homeproxy/windows'),
      (call) async {
        if (call.method == 'serviceStatus') {
          scm++;
          return 'running';
        }
        pipe++;
        return jsonEncode({'version': 1, 'ok': true, 'status': 'Durduruldu'});
      },
    );
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pumpWidget(const MaterialApp(home: WindowsAgentScreen()));
    await tester.pump();
    expect([scm, pipe], [1, 1]);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.inactive);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.hidden);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
    await tester.pump(const Duration(seconds: 6));
    expect([scm, pipe], [1, 1]);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.hidden);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.inactive);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pump();
    expect([scm, pipe], [2, 2]);
    await tester.pumpWidget(const SizedBox());
  });
}
