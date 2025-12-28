defmodule EventodbKit.TestSupport.EventDispatcher do
  @moduledoc """
  Type-safe event dispatcher for test consumers.
  
  Uses the EventodbKit.EventDispatcher API to register generated event modules.
  """
  
  use EventodbKit.EventDispatcher
  
  # Register all generated event modules
  register "PartnershipApplicationSubmitted", Events.PartnershipApplicationSubmitted
  register "PartnershipActivated", Events.PartnershipActivated
  register "ClassJoinRequested", Events.ClassJoinRequested
  register "ClassMembershipAccepted", Events.ClassMembershipAccepted
  register "ClassMembershipRejected", Events.ClassMembershipRejected
  register "ContentErrorReported", Events.ContentErrorReported
  register "StudentProgressMilestoneReached", Events.StudentProgressMilestoneReached
end
