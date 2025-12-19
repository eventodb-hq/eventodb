defmodule MessagedbEx.Types do
  @moduledoc """
  Type definitions for MessageDB client.
  """

  @type message :: %{
          type: String.t(),
          data: map(),
          metadata: map() | nil
        }

  @type write_options :: %{
          optional(:id) => String.t(),
          optional(:expected_version) => integer()
        }

  @type write_result :: %{
          position: non_neg_integer(),
          global_position: non_neg_integer()
        }

  @type get_stream_options :: %{
          optional(:position) => non_neg_integer(),
          optional(:global_position) => non_neg_integer(),
          optional(:batch_size) => integer()
        }

  @type get_category_options :: %{
          optional(:position) => non_neg_integer(),
          optional(:global_position) => non_neg_integer(),
          optional(:batch_size) => integer(),
          optional(:correlation) => String.t(),
          optional(:consumer_group) => %{
            member: non_neg_integer(),
            size: pos_integer()
          }
        }

  @type namespace_options :: %{
          optional(:description) => String.t(),
          optional(:token) => String.t()
        }

  # Stream message: [id, type, position, globalPosition, data, metadata, time]
  @type stream_message ::
          {String.t(), String.t(), non_neg_integer(), non_neg_integer(), map(), map() | nil,
           String.t()}
          | list()

  # Category message: [id, streamName, type, position, globalPosition, data, metadata, time]
  @type category_message ::
          {String.t(), String.t(), String.t(), non_neg_integer(), non_neg_integer(), map(),
           map() | nil, String.t()}
          | list()
end
