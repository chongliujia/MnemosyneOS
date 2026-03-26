# mnemosyne-sdk

Python client SDK for MnemosyneOS Memory VFS API.

## Install

```bash
pip install -e .
```

## Quickstart

```python
from mnemosyne_sdk import CreateCardRequest, MnemosyneClient

client = MnemosyneClient(base_url="http://127.0.0.1:8080")
card = client.create_card(
    CreateCardRequest(
        card_id="evt-1",
        card_type="event",
        content={"text": "User likes black coffee"},
    )
)
print(card.card_id, card.version)
client.close()
```
