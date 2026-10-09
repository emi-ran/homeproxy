import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

import '../app/lib/main.dart';

void main() {
  for (final staleError in [false, true]) {
    testWidgets(
      'in-flight ${staleError ? "error" : "success"} across resume is stale and never overlaps',
      (tester) async {
        final pending = Completer<String>();
        final fresh = Completer<String>();
        var polls = 0;
        tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
          const MethodChannel('homeproxy/agent'),
          (call) async {
            if (call.method == 'loadSettings') return <String, dynamic>{};
            if (call.method != 'status') return null;
            polls++;
            return polls == 1 ? pending.future : fresh.future;
          },
        );
        tester.binding.handleAppLifecycleStateChanged(
          AppLifecycleState.resumed,
        );
        await tester.pumpWidget(const MaterialApp(home: AgentScreen()));
        await tester.pump();
        await tester.pump(const Duration(seconds: 3));
        expect(polls, 1, reason: 'timer must not overlap an in-flight poll');
        tester.binding.handleAppLifecycleStateChanged(
          AppLifecycleState.inactive,
        );
        tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.hidden);
        tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
        await tester.pump(const Duration(seconds: 3));
        expect(polls, 1);
        tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.hidden);
        tester.binding.handleAppLifecycleStateChanged(
          AppLifecycleState.inactive,
        );
        tester.binding.handleAppLifecycleStateChanged(
          AppLifecycleState.resumed,
        );
        await tester.pump();
        expect(polls, 1, reason: 'resume must wait for the older request');
        if (staleError) {
          pending.completeError(PlatformException(code: 'STALE'));
        } else {
          pending.complete('stale');
        }
        await tester.pump();
        await tester.pump();
        expect(find.text('stale'), findsNothing);
        expect(find.text('Android servisine erişilemiyor'), findsNothing);
        final dynamic screenState = tester.state(find.byType(AgentScreen));
        expect(
          screenState.error,
          isNull,
          reason: 'stale error must not be stored offscreen',
        );
        expect(
          polls,
          2,
          reason: 'stale completion must trigger immediate fresh poll',
        );
        await tester.pump(const Duration(seconds: 3));
        expect(polls, 2, reason: 'fresh request also must not overlap');
        fresh.complete('fresh');
        await tester.pump();
        await tester.pump();
        expect(find.text('fresh'), findsOneWidget);
        await tester.pumpWidget(const SizedBox());
      },
    );
  }
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
