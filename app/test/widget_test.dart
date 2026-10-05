import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:homeproxy_agent/main.dart';

void main() {
  testWidgets('Agent form validates empty settings', (tester) async {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(
          const MethodChannel('homeproxy/agent'),
          (_) async => 'Durduruldu',
        );
    await tester.pumpWidget(const HomeProxyApp());
    await tester.scrollUntilVisible(
      find.text('Bağlantıyı başlat'),
      250,
      scrollable: find
          .descendant(
            of: find.byType(ListView),
            matching: find.byType(Scrollable),
          )
          .first,
    );
    await tester.tap(find.text('Bağlantıyı başlat'));
    await tester.pump();
    expect(find.text('Sunucu adresi gerekli'), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
  });
}
