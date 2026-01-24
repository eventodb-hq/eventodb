Currently, each of our consumers is polling Evento DB individually. 
Now imagine we have multiple consumers in a single process. Each service can have multiple processes and we also have multiple services. 
And now imagine all those individual consumers and all those processors for all those services are polling Evento DB once per second. 
This is this is very wasteful. 
And it also makes it hard to spot any issues because anytime EventoDB server has to handle hundreds, if not thousands of requests per second just to keep things updated even if nothing has happened 

This is not very efficient, right? 


I imagine we can reduce it to have a single subscribing global consumer. 

We added subscription to the conceptual All stream. 
That means we can subscribe to all events in a single namespace. 
And then we get pokes for each of those events. 


That means we can have a single gen server that is responsible to keep things updated locally. 
And this gen server will get all the notifications and will distribute them locally to the relevant consumers. 
And also we can tell those consumers to keep polling the local gen server instead of event to db. 
That way, we have reduced quite some noise. 
And we divided responsibilities. 
To make this change less disruptive, we can make the current consumers configurable by providing them a polling function. 

Meaning, if we do not provide any polling function, then they default to the current behavior, which is polling evento db for their category. 

But if we provide a polling function, then they just execute this function and well hopefully get the same responses, just not directly from EventoDB, but from this local gen server that subscribes to all the changes. 

Do you understand what I mean? 

I ask you to look through the code base, especially in the Elixir SDK and Propose how we can implement it. 
Do not implement anything yet, I want to be sure that you really understand the issue. 