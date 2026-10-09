import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

import '../app/lib/main.dart';

void main() {
  testWidgets('Android polling stops offscreen and resumes immediately', (
    tester,
  ) async {
    var polls = 0;
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      const MethodChannel('homeproxy/agent'),
      (call) async {
        if (call.method == 'status') {
          polls++;
          return 'Durduruldu';
        }
        if (call.method == 'loadSettings') return <String, dynamic>{};
        return null;
      },
    );
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pumpWidget(const MaterialApp(home: AgentScreen()));
    await tester.pump();
    expect(polls, 1);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.inactive);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.hidden);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
    await tester.pump(const Duration(seconds: 3));
    expect(polls, 1, reason: 'paused UI must not poll local service');
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.hidden);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.inactive);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pump();
    expect(polls, 2, reason: 'resume must refresh immediately');
    await tester.pumpWidget(const SizedBox());
  });

  testWidgets('unchanged Android status does not rebuild screen', (
    tester,
  ) async {
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      const MethodChannel('homeproxy/agent'),
      (call) async {
        if (call.method == 'status') return 'Durduruldu';
        if (call.method == 'loadSettings') return <String, dynamic>{};
        return null;
      },
    );
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pumpWidget(const MaterialApp(home: AgentScreen()));
    await tester.pump();
    final before = tester.widget<Text>(find.text('Durduruldu'));
    await tester.pump(const Duration(seconds: 1));
    await tester.pump();
    expect(
      identical(before, tester.widget<Text>(find.text('Durduruldu'))),
      isTrue,
    );
    await tester.pumpWidget(const SizedBox());
  });
}
