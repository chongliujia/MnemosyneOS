from __future__ import annotations

from datetime import datetime
from typing import Any, Dict, List, Optional

from pydantic import BaseModel, Field


class EvidenceRef(BaseModel):
    card_id: str
    snippet: Optional[str] = None
    hash: Optional[str] = None


class Provenance(BaseModel):
    agent_id: Optional[str] = None
    source: Optional[str] = None
    confidence: Optional[float] = None


class ActivationState(BaseModel):
    score: float = 1.0
    last_access_at: Optional[datetime] = None
    decay_policy: Optional[str] = None


class Card(BaseModel):
    card_id: str
    card_type: str
    created_at: datetime
    valid_from: Optional[datetime] = None
    valid_to: Optional[datetime] = None
    version: int
    prev_version_id: Optional[str] = None
    status: str
    content: Dict[str, Any] = Field(default_factory=dict)
    evidence_refs: List[EvidenceRef] = Field(default_factory=list)
    provenance: Provenance = Field(default_factory=Provenance)
    activation_state: ActivationState = Field(default_factory=ActivationState)


class Edge(BaseModel):
    edge_id: str
    from_card_id: str
    to_card_id: str
    edge_type: str
    weight: Optional[float] = None
    confidence: Optional[float] = None
    valid_from: Optional[datetime] = None
    valid_to: Optional[datetime] = None
    evidence_refs: List[EvidenceRef] = Field(default_factory=list)
    created_at: datetime


class CreateCardRequest(BaseModel):
    card_id: str
    card_type: str
    content: Dict[str, Any] = Field(default_factory=dict)
    valid_from: Optional[datetime] = None
    valid_to: Optional[datetime] = None
    evidence_refs: List[EvidenceRef] = Field(default_factory=list)
    provenance: Provenance = Field(default_factory=Provenance)


class UpdateCardRequest(BaseModel):
    content: Optional[Dict[str, Any]] = None
    valid_from: Optional[datetime] = None
    valid_to: Optional[datetime] = None
    status: Optional[str] = None
    evidence_refs: Optional[List[EvidenceRef]] = None
    provenance: Optional[Provenance] = None


class CreateEdgeRequest(BaseModel):
    edge_id: str
    from_card_id: str
    to_card_id: str
    edge_type: str
    weight: Optional[float] = None
    confidence: Optional[float] = None
    valid_from: Optional[datetime] = None
    valid_to: Optional[datetime] = None
    evidence_refs: List[EvidenceRef] = Field(default_factory=list)


class QueryResponse(BaseModel):
    cards: List[Card] = Field(default_factory=list)
    edges: List[Edge] = Field(default_factory=list)
