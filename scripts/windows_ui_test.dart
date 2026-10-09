import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

import '../app/lib/windows_agent.dart';

void main() {
  for (final command in ['set_config', 'disconnect', 'serviceCommand']) {
    testWidgets('resume defers polling until $command finishes', (
      tester,
    ) async {
      await tester.binding.setSurfaceSize(const Size(1000, 1800));
      addTearDown(() => tester.binding.setSurfaceSize(null));
      final pending = Completer<String>();
      var active = false, overlap = 0, polls = 0, commands = 0;
      String reply(String status) => jsonEncode({
        'version': 1,
        'ok': true,
        'status': status,
        'config': {
          'address': 'proxy.example.com:4433',
          'id': 'test-pc',
          'fingerprint': List.filled(64, 'a').join(),
          'enabled': true,
          'insecure': false,
        },
      });
      tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
        const MethodChannel('homeproxy/windows'),
        (call) async {
          final op = call.method == 'request'
              ? (jsonDecode(call.arguments as String) as Map)['op']
              : call.method;
          if (op == command) {
            commands++;
            active = true;
            try {
              return await pending.future;
            } finally {
              active = false;
            }
          }
          if (active) {
            overlap++;
            throw PlatformException(code: 'busy', message: 'bridge busy');
          }
          if (op == 'serviceStatus') {
            polls++;
            return 'running';
          }
          return reply('fresh');
        },
      );
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
      await tester.pumpWidget(const MaterialApp(home: WindowsAgentScreen()));
      await tester.pump();
      final label = switch (command) {
        'set_config' => 'Ayarları kaydet',
        'disconnect' => 'Bağlantıyı kes',
        _ => 'Windows servisini durdur',
      };
      await tester.tap(find.text(label));
      await tester.pump();
      if (command == 'serviceCommand') {
        await tester.tap(find.text('Onayla (UAC)'));
        await tester.pump();
      }
      expect(commands, 1);
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.inactive);
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.hidden);
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.hidden);
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.inactive);
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
      await tester.pump();
      await tester.pump(const Duration(seconds: 6));
      expect(overlap, 0);
      expect(polls, 1);
      pending.complete(
        command == 'serviceCommand' ? 'ok' : reply('command done'),
      );
      await tester.pump();
      await tester.pump();
      expect(polls, greaterThan(1));
      expect(commands, 1);
      expect(find.text('bridge busy'), findsNothing);
      expect(find.text('Tünel: fresh'), findsOneWidget);
      await tester.pumpWidget(const SizedBox());
    });
  }
  testWidgets('stale pipe reply is ignored and resume never overlaps polls', (
    tester,
  ) async {
    final pending = Completer<String>();
    var requests = 0;
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      const MethodChannel('homeproxy/windows'),
      (call) async {
        if (call.method == 'serviceStatus') return 'running';
        requests++;
        if (requests == 1) return pending.future;
        return jsonEncode({'version': 1, 'ok': true, 'status': 'fresh'});
      },
    );
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pumpWidget(const MaterialApp(home: WindowsAgentScreen()));
    await tester.pump();
    await tester.pump(const Duration(seconds: 6));
    expect(requests, 1);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.inactive);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.hidden);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
    pending.complete(jsonEncode({'version': 1, 'ok': true, 'status': 'stale'}));
    await tester.pump();
    expect(find.text('Tünel: stale'), findsNothing);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.hidden);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.inactive);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pump();
    expect(requests, 2);
    expect(find.text('Tünel: fresh'), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
  });
  for (final fail in [false, true]) {
    testWidgets(
      'resume before old poll completes ignores stale ${fail ? "error" : "success"}',
      (tester) async {
        final pending = Completer<String>();
        var requests = 0, active = 0, maxActive = 0;
        tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
          const MethodChannel('homeproxy/windows'),
          (call) async {
            if (call.method == 'serviceStatus') return 'running';
            requests++;
            active++;
            if (active > maxActive) maxActive = active;
            try {
              if (requests == 1) return await pending.future;
              return jsonEncode({'version': 1, 'ok': true, 'status': 'fresh'});
            } finally {
              active--;
            }
          },
        );
        tester.binding.handleAppLifecycleStateChanged(
          AppLifecycleState.resumed,
        );
        await tester.pumpWidget(const MaterialApp(home: WindowsAgentScreen()));
        await tester.pump();
        tester.binding.handleAppLifecycleStateChanged(
          AppLifecycleState.inactive,
        );
        tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.hidden);
        tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
        tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.hidden);
        tester.binding.handleAppLifecycleStateChanged(
          AppLifecycleState.inactive,
        );
        tester.binding.handleAppLifecycleStateChanged(
          AppLifecycleState.resumed,
        );
        await tester.pump(const Duration(seconds: 6));
        expect(requests, 1);
        if (fail) {
          pending.completeError(
            PlatformException(code: 'ipc', message: 'stale error'),
          );
        } else {
          pending.complete(
            jsonEncode({'version': 1, 'ok': true, 'status': 'stale'}),
          );
        }
        await tester.pump();
        await tester.pump();
        expect(requests, 2);
        expect(maxActive, 1);
        final dynamic state = tester.state(find.byType(WindowsAgentScreen));
        expect(state.status, 'fresh');
        expect(state.error, isNull);
        expect(find.text('Tünel: fresh'), findsOneWidget);
        await tester.pumpWidget(const SizedBox());
      },
    );
  }
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
