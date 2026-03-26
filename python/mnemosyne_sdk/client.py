from __future__ import annotations

from datetime import datetime
from typing import Optional

import httpx

from .models import (
    Card,
    CreateCardRequest,
    CreateEdgeRequest,
    Edge,
    QueryResponse,
    UpdateCardRequest,
)


class MnemosyneClient:
    def __init__(self, base_url: str = "http://127.0.0.1:8080", timeout: float = 10.0):
        self._client = httpx.Client(base_url=base_url, timeout=timeout)

    def close(self) -> None:
        self._client.close()

    def create_card(self, req: CreateCardRequest) -> Card:
        resp = self._client.put("/cards", json=req.model_dump(mode="json"))
        resp.raise_for_status()
        return Card.model_validate(resp.json())

    def update_card(self, card_id: str, req: UpdateCardRequest) -> Card:
        payload = req.model_dump(mode="json", exclude_none=True)
        resp = self._client.patch(f"/cards/{card_id}", json=payload)
        resp.raise_for_status()
        return Card.model_validate(resp.json())

    def create_edge(self, req: CreateEdgeRequest) -> Edge:
        resp = self._client.post("/edges", json=req.model_dump(mode="json"))
        resp.raise_for_status()
        return Edge.model_validate(resp.json())

    def query(self, card_id: Optional[str] = None, card_type: Optional[str] = None, as_of: Optional[datetime] = None) -> QueryResponse:
        params = {}
        if card_id:
            params["card_id"] = card_id
        if card_type:
            params["card_type"] = card_type
        if as_of:
            params["as_of"] = as_of.isoformat()
        resp = self._client.get("/query", params=params)
        resp.raise_for_status()
        return QueryResponse.model_validate(resp.json())


class AsyncMnemosyneClient:
    def __init__(self, base_url: str = "http://127.0.0.1:8080", timeout: float = 10.0):
        self._client = httpx.AsyncClient(base_url=base_url, timeout=timeout)

    async def aclose(self) -> None:
        await self._client.aclose()

    async def create_card(self, req: CreateCardRequest) -> Card:
        resp = await self._client.put("/cards", json=req.model_dump(mode="json"))
        resp.raise_for_status()
        return Card.model_validate(resp.json())

    async def update_card(self, card_id: str, req: UpdateCardRequest) -> Card:
        payload = req.model_dump(mode="json", exclude_none=True)
        resp = await self._client.patch(f"/cards/{card_id}", json=payload)
        resp.raise_for_status()
        return Card.model_validate(resp.json())

    async def create_edge(self, req: CreateEdgeRequest) -> Edge:
        resp = await self._client.post("/edges", json=req.model_dump(mode="json"))
        resp.raise_for_status()
        return Edge.model_validate(resp.json())

    async def query(self, card_id: Optional[str] = None, card_type: Optional[str] = None, as_of: Optional[datetime] = None) -> QueryResponse:
        params = {}
        if card_id:
            params["card_id"] = card_id
        if card_type:
            params["card_type"] = card_type
        if as_of:
            params["as_of"] = as_of.isoformat()
        resp = await self._client.get("/query", params=params)
        resp.raise_for_status()
        return QueryResponse.model_validate(resp.json())
