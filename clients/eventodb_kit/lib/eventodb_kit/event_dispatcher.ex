defmodule EventodbKit.EventDispatcher do
  @moduledoc """
  Type-safe event dispatcher for integrating code-generated event modules.
  
  This module provides a clean API for routing and validating events using
  generated event schemas (from eventodb-docs/code-gen).
  
  ## Usage
  
      # Define your event registry
      defmodule MyApp.Events do
        use EventodbKit.EventDispatcher
        
        # Register generated event modules
        register "UserCreated", MyApp.Events.UserCreated
        register "UserUpdated", MyApp.Events.UserUpdated
        register "OrderPlaced", MyApp.Events.OrderPlaced
      end
      
      # Use in consumers
      defmodule MyApp.MyConsumer do
        use EventodbKit.Consumer
        
        def handle_message(message, state) do
          MyApp.Events.dispatch(message["type"], message["data"], &handle_event/2)
        end
        
        # Pattern match on event modules for type safety
        defp handle_event(MyApp.Events.UserCreated, event) do
          # event is validated and has struct fields
          IO.puts("User created: \#{event.user_id}")
          :ok
        end
        
        defp handle_event(MyApp.Events.OrderPlaced, event) do
          IO.puts("Order placed: \#{event.order_id}")
          :ok
        end
      end
  """
  
  @doc """
  Use this module to create an event dispatcher with type-safe event routing.
  """
  defmacro __using__(_opts) do
    quote do
      import EventodbKit.EventDispatcher, only: [register: 2]
      
      Module.register_attribute(__MODULE__, :event_registry, accumulate: true)
      
      @before_compile EventodbKit.EventDispatcher
    end
  end
  
  @doc """
  Register an event type with its corresponding module.
  
  The module should be a generated event schema with `validate!/1` function.
  """
  defmacro register(event_type, event_module) do
    quote do
      @event_registry {unquote(event_type), unquote(event_module)}
    end
  end
  
  defmacro __before_compile__(env) do
    registry = Module.get_attribute(env.module, :event_registry)
    
    # Build the event map at compile time
    event_map =
      registry
      |> Enum.reverse()
      |> Enum.into(%{})
      |> Macro.escape()
    
    quote do
      @doc """
      Dispatches an event to a handler with validation.
      
      ## Parameters
      
        * `event_type` - The event type name (string)
        * `data` - The event data (map or struct)
        * `handler` - A 2-arity function that receives (event_module, validated_event)
      
      ## Returns
      
        * `{:ok, result}` - Handler result on success
        * `{:error, :unknown_event}` - Event type not registered
        * `{:error, changeset}` - Validation failed
      """
      def dispatch(event_type, data, handler) when is_function(handler, 2) do
        case Map.get(unquote(event_map), event_type) do
          nil ->
            {:error, :unknown_event}
          
          event_module ->
            with {:ok, validated} <- event_module.validate!(data) do
              result = handler.(event_module, validated)
              {:ok, result}
            end
        end
      end
      
      @doc """
      Validates event data without dispatching.
      
      ## Parameters
      
        * `event_type` - The event type name (string)
        * `data` - The event data (map or struct)
      
      ## Returns
      
        * `{:ok, validated_event}` - Validated event struct
        * `{:error, :unknown_event}` - Event type not registered
        * `{:error, changeset}` - Validation failed
      """
      def validate(event_type, data) do
        case Map.get(unquote(event_map), event_type) do
          nil ->
            {:error, :unknown_event}
          
          event_module ->
            event_module.validate!(data)
        end
      end
      
      @doc """
      Returns the list of registered event types.
      """
      def registered_events do
        Map.keys(unquote(event_map))
      end
      
      @doc """
      Returns the event module for a given event type.
      """
      def event_module(event_type) do
        Map.get(unquote(event_map), event_type)
      end
      
      @doc """
      Returns the event category for a given event type.
      """
      def event_category(event_type) do
        case Map.get(unquote(event_map), event_type) do
          nil -> {:error, :unknown_event}
          event_module -> {:ok, event_module.category()}
        end
      end
    end
  end
  
  @doc """
  Helper to publish validated events with automatic stream naming.
  
  ## Parameters
  
    * `client` - EventodbEx client
    * `event_type` - The event type name
    * `data` - The event data
    * `event_module` - The event module (must have `stream_name/1`)
    * `opts` - Additional options for publishing
  
  ## Returns
  
    * `{:ok, result, client}` - Published successfully
    * `{:error, changeset}` - Validation failed
    * `{:error, reason}` - Publishing failed
  """
  def publish_event(client, event_type, data, event_module, opts \\ %{}) do
    with {:ok, _validated} <- event_module.validate!(data),
         stream <- event_module.stream_name(data) do
      EventodbEx.stream_write(
        client,
        stream,
        Map.merge(
          %{
            type: event_type,
            data: data
          },
          opts
        )
      )
    end
  end
end
