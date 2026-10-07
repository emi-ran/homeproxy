#!/usr/bin/env python3
"""SDK-free integration contracts, not Android compilation/device proof."""
from pathlib import Path
import unittest
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / 'app/android/app/src/main'
KOTLIN = SRC / 'kotlin/com/homeproxy/homeproxy_agent'
A = '{http://schemas.android.com/apk/res/android}'

class TileContracts(unittest.TestCase):
    def test_system_tile_registration(self):
        services = ET.parse(SRC / 'AndroidManifest.xml').findall('application/service')
        tile = next((s for s in services if s.get(A + 'name') == '.AgentTileService'), None)
        self.assertIsNotNone(tile, 'Quick Settings service missing')
        assert tile is not None
        self.assertEqual(tile.get(A + 'permission'), 'android.permission.BIND_QUICK_SETTINGS_TILE')
        self.assertEqual(tile.get(A + 'exported'), 'true')
        self.assertEqual(tile.find('intent-filter/action').get(A + 'name'), 'android.service.quicksettings.action.QS_TILE')
        self.assertTrue((SRC / 'res/drawable/ic_tunnel.xml').exists())

    def test_connected_only_active_and_running_can_stop(self):
        tile = (KOTLIN / 'AgentTileService.kt').read_text()
        self.assertIn('AgentService.status == "Bağlı"', tile)
        self.assertIn('Tile.STATE_ACTIVE', tile)
        self.assertIn('Tile.STATE_INACTIVE', tile)
        self.assertIn('if (AgentService.running)', tile)
        self.assertIn('stopService(', tile)
        self.assertIn('AgentSettings(this).load()', tile)
        self.assertIn('override fun onStartListening()', tile)
        self.assertIn('unlockAndRun', tile)

    def test_service_publishes_callbacks_and_lifecycle(self):
        service = (KOTLIN / 'AgentService.kt').read_text()
        self.assertIn('agent.setStatusListener', service)
        self.assertIn('override fun onStatus', service)
        self.assertIn('AgentTileService.refresh(this)', service)
        self.assertIn('if (!destroyed && next !=', service)
        self.assertIn('running = false', service)
        activity = (KOTLIN / 'MainActivity.kt').read_text()
        self.assertNotIn('AgentService.status =', activity)

if __name__ == '__main__':
    unittest.main()
