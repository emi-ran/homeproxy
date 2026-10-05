import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:homeproxy_agent/main.dart';

void main() {
  const channel = MethodChannel('homeproxy/windows');
  late List<MethodCall> calls;
  late String state;
  late Map<String, dynamic>? config;
  setUp(() {
    calls = [];
    state = 'running';
    config = {
      'address': 'example.com:4433',
      'id': 'ev-pc',
      'fingerprint': List.filled(64, 'a').join(),
      'insecure': false,
      'enabled': false,
    };
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
          calls.add(call);
          if (call.method == 'serviceStatus') return state;
          if (call.method == 'serviceCommand') return 'ok';
          final req =
              jsonDecode(call.arguments as String) as Map<String, dynamic>;
          if (req['op'] == 'set_config') {
            config = Map<String, dynamic>.from(req['config'] as Map);
          }
          if (req['op'] == 'connect') config!['enabled'] = true;
          if (req['op'] == 'disconnect') config!['enabled'] = false;
          return jsonEncode({
            'version': 1,
            'ok': true,
            'status': 'Durduruldu',
            'config': ?config,
          });
        });
  });
  tearDown(() {
    debugDefaultTargetPlatformOverride = null;
  });
  Future<void> open(WidgetTester tester) async {
    debugDefaultTargetPlatformOverride = TargetPlatform.windows;
    tester.view.physicalSize = const Size(1000, 1800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(const HomeProxyApp());
    await tester.pumpAndSettle();
  }

  Future<void> close(WidgetTester tester) async {
    await tester.pumpWidget(const SizedBox());
    debugDefaultTargetPlatformOverride = null;
  }

  testWidgets('Windows restores redacted settings without connecting', (
    tester,
  ) async {
    await open(tester);
    expect(find.text('WINDOWS AGENT'), findsOneWidget);
    expect(find.text('example.com:4433'), findsOneWidget);
    expect(
      tester.widget<SwitchListTile>(find.byType(SwitchListTile)).value,
      false,
    );
    expect(
      calls
          .where((c) => c.method == 'request')
          .map((c) => jsonDecode(c.arguments as String)['op']),
      ['status'],
    );
    await close(tester);
  });
  testWidgets(
    'save retains blank token and intent; disconnect is not SCM stop',
    (tester) async {
      await open(tester);
      await tester.tap(find.text('Ayarları kaydet'));
      await tester.pumpAndSettle();
      var req = jsonDecode(calls.last.arguments as String);
      expect(req['op'], 'set_config');
      expect(req['config']['token'], '');
      expect(req['config']['enabled'], false);
      expect(req['config']['insecure'], false);
      await tester.tap(find.text('Kayıtlı ayarlarla bağlan'));
      await tester.pumpAndSettle();
      expect(jsonDecode(calls.last.arguments as String)['op'], 'connect');
      await tester.tap(find.text('Bağlantıyı kes'));
      await tester.pumpAndSettle();
      expect(jsonDecode(calls.last.arguments as String)['op'], 'disconnect');
      expect(calls.any((c) => c.method == 'serviceCommand'), false);
      await close(tester);
    },
  );
  testWidgets('missing service install needs consent and never auto-starts', (
    tester,
  ) async {
    state = 'missing';
    await open(tester);
    expect(find.text('Windows servisi kurulu değil'), findsOneWidget);
    await tester.tap(find.text('Servisi kur'));
    await tester.pumpAndSettle();
    expect(calls.any((c) => c.method == 'serviceCommand'), false);
    await tester.tap(find.text('Vazgeç'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Servisi kur'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Onayla (UAC)'));
    await tester.pumpAndSettle();
    expect(
      calls.where((c) => c.method == 'serviceCommand').map((c) => c.arguments),
      ['install'],
    );
    expect(calls.any((c) => c.method == 'request'), false);
    await close(tester);
  });
  testWidgets('stopped and pending services cannot send settings', (
    tester,
  ) async {
    state = 'stopped';
    await open(tester);
    expect(find.text('Windows servisi durdurulmuş'), findsOneWidget);
    expect(find.text('Servisi başlat'), findsOneWidget);
    expect(find.text('Servisi kaldır'), findsOneWidget);
    expect(
      tester
          .widget<FilledButton>(
            find.widgetWithText(FilledButton, 'Kaydet ve bağlan'),
          )
          .onPressed,
      null,
    );
    state = 'pending';
    await tester.pump(const Duration(seconds: 2));
    await tester.pumpAndSettle();
    expect(find.text('Windows servisi geçiş halinde; bekle'), findsOneWidget);
    await close(tester);
  });
  testWidgets('insecure requires warning acceptance; initial token required', (
    tester,
  ) async {
    config = null;
    await open(tester);
    await tester.tap(find.byType(SwitchListTile));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Vazgeç'));
    await tester.pumpAndSettle();
    expect(
      tester.widget<SwitchListTile>(find.byType(SwitchListTile)).value,
      false,
    );
    await tester.tap(find.byType(SwitchListTile));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Riski kabul et'));
    await tester.pumpAndSettle();
    expect(
      tester.widget<SwitchListTile>(find.byType(SwitchListTile)).value,
      true,
    );
    await tester.tap(find.text('Kaydet ve bağlan'));
    await tester.pumpAndSettle();
    expect(find.text('Token 16–4096 bayt olmalı'), findsOneWidget);
    expect(
      calls
          .where((c) => c.method == 'request')
          .every((c) => jsonDecode(c.arguments as String)['op'] == 'status'),
      true,
    );
    await close(tester);
  });
  testWidgets('access denial surfaces explicit error', (tester) async {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
          if (call.method == 'serviceStatus') return 'running';
          throw PlatformException(
            code: 'windows_agent',
            message:
                'Pipe access denied. Sign in as authorized installation user.',
          );
        });
    await open(tester);
    expect(
      find.text('Pipe access denied. Sign in as authorized installation user.'),
      findsOneWidget,
    );
    await close(tester);
  });

  testWidgets('UAC cancellation is shown, never treated as install success', (
    tester,
  ) async {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
          if (call.method == 'serviceStatus') return 'missing';
          if (call.method == 'serviceCommand') {
            throw PlatformException(
              code: 'windows_agent',
              message:
                  'UAC cancelled. Service unchanged. Approve UAC to continue.',
            );
          }
          throw StateError('Unexpected pipe request');
        });
    await open(tester);
    await tester.tap(find.text('Servisi kur'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Onayla (UAC)'));
    await tester.pumpAndSettle();
    expect(
      find.text('UAC cancelled. Service unchanged. Approve UAC to continue.'),
      findsOneWidget,
    );
    expect(find.text('Windows servisi kurulu değil'), findsOneWidget);
    await close(tester);
  });

  for (final action in ['start', 'stop', 'uninstall']) {
    testWidgets('$action requires separate SCM consent', (tester) async {
      state = action == 'stop' ? 'running' : 'stopped';
      await open(tester);
      final label = {
        'start': 'Servisi başlat',
        'stop': 'Windows servisini durdur',
        'uninstall': 'Servisi kaldır',
      }[action]!;
      await tester.tap(find.text(label));
      await tester.pumpAndSettle();
      expect(calls.any((c) => c.method == 'serviceCommand'), false);
      await tester.tap(find.text('Vazgeç'));
      await tester.pumpAndSettle();
      await tester.tap(find.text(label));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Onayla (UAC)'));
      await tester.pumpAndSettle();
      expect(
        calls
            .where((c) => c.method == 'serviceCommand')
            .map((c) => c.arguments),
        [action],
      );
      await close(tester);
    });
  }

  testWidgets('save while enabled preserves intent and save-connect enables', (
    tester,
  ) async {
    config!['enabled'] = true;
    await open(tester);
    await tester.tap(find.text('Ayarları kaydet'));
    await tester.pumpAndSettle();
    expect(config!['enabled'], true);
    await tester.tap(find.text('Bağlantıyı kes'));
    await tester.pumpAndSettle();
    expect(config!['enabled'], false);
    await tester.tap(find.text('Kaydet ve bağlan'));
    await tester.pumpAndSettle();
    expect(config!['enabled'], true);
    await close(tester);
  });
}
